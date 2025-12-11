package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"webhook-processor/config"
	"webhook-processor/internal/models"
	"webhook-processor/pkg/metrics"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Publisher interface {
	Publish(event models.WebhookEvent) error
	HealthCheck(ctx context.Context) error
	Close() error
}

type RabbitMQ struct {
	conn         *amqp.Connection
	ch           *amqp.Channel
	exchangeName string
	logger       *zap.Logger
	queueName    string
	config       config.RabbitMQConfig
}

// StartMetricsUpdater starts a goroutine to periodically update queue metrics
func (r *RabbitMQ) StartMetricsUpdater(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if queue, err := r.ch.QueueInspect(r.queueName); err == nil {
					metrics.WebhookQueueSize.WithLabelValues("all").Set(float64(queue.Messages))
				}
			}
		}
	}()
}

func NewRabbitMQ(rabbitCfg config.RabbitMQConfig, logger *zap.Logger) (*RabbitMQ, error) {
	conn, err := amqp.Dial(rabbitCfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %v", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %v", err)
	}

	q, err := DeclareTopology(ch, rabbitCfg, logger)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, err
	}

	return &RabbitMQ{
		conn:         conn,
		ch:           ch,
		exchangeName: rabbitCfg.Exchange,
		logger:       logger,
		queueName:    q.Name,
		config:       rabbitCfg,
	}, nil
}

func (r *RabbitMQ) Publish(event models.WebhookEvent) error {
	const maxRetries = 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Check if connection is closed; reconnect if needed
		if r.conn == nil || r.conn.IsClosed() || r.ch == nil || r.ch.IsClosed() {
			r.logger.Warn("RabbitMQ connection lost, attempting reconnect",
				zap.Int("attempt", attempt+1),
			)
			if err := r.reconnect(); err != nil {
				lastErr = err
				backoffDuration := time.Duration(1<<uint(attempt)) * time.Second // exponential backoff
				r.logger.Error("Reconnect failed, backing off",
					zap.Int("attempt", attempt+1),
					zap.Duration("backoff", backoffDuration),
					zap.Error(err),
				)
				time.Sleep(backoffDuration)
				continue
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		body, err := json.Marshal(event)
		cancel()

		if err != nil {
			return fmt.Errorf("failed to marshal event: %v", err)
		}

		// Add headers with the client ID for the worker to use
		headers := make(amqp.Table)
		headers["webhook_id"] = event.WebhookID
		headers["webhook_type"] = event.WebhookType
		headers["client_id"] = event.ClientID

		// Publish to all queues bound to this exchange
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		err = r.ch.PublishWithContext(ctx,
			r.exchangeName,
			"",    // routing key
			false, // mandatory
			false, // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				Headers:      headers,
				Body:         body,
				DeliveryMode: amqp.Persistent,
			})
		cancel()

		if err == nil {
			// Log publish success for tracing
			if r.logger != nil {
				r.logger.Info("Published message",
					zap.String("client_id", event.ClientID),
					zap.String("event", event.Event),
					zap.String("webhook_id", event.WebhookID),
					zap.String("exchange", r.exchangeName),
				)
			}
			return nil
		}

		lastErr = err
		r.logger.Warn("Publish failed, will retry",
			zap.Int("attempt", attempt+1),
			zap.Error(err),
		)

		backoffDuration := time.Duration(1<<uint(attempt)) * time.Second
		time.Sleep(backoffDuration)
	}

	return fmt.Errorf("failed to publish message after %d attempts: %w", maxRetries, lastErr)
}

func (r *RabbitMQ) reconnect() error {
	// Close existing connection/channel
	if r.ch != nil && !r.ch.IsClosed() {
		r.ch.Close()
	}
	if r.conn != nil && !r.conn.IsClosed() {
		r.conn.Close()
	}

	// Dial new connection
	conn, err := amqp.Dial(r.config.URL)
	if err != nil {
		return fmt.Errorf("failed to reconnect to RabbitMQ: %v", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open new channel: %v", err)
	}

	// Re-declare topology (idempotent)
	_, err = DeclareTopology(ch, r.config, r.logger)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("failed to redeclare topology: %v", err)
	}

	r.conn = conn
	r.ch = ch
	r.logger.Info("Reconnected to RabbitMQ")
	return nil
}

func (r *RabbitMQ) HealthCheck(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if r.conn == nil || r.conn.IsClosed() {
		return errors.New("rabbitmq connection closed")
	}
	if r.ch == nil || r.ch.IsClosed() {
		return errors.New("rabbitmq channel closed")
	}
	return nil
}

func (r *RabbitMQ) Close() error {
	if err := r.ch.Close(); err != nil {
		r.logger.Error("Failed to close channel", zap.Error(err))
	}
	if err := r.conn.Close(); err != nil {
		r.logger.Error("Failed to close connection", zap.Error(err))
	}
	return nil
}

func (r *RabbitMQ) DeclareClientQueue(clientID string) error {
	queueName := fmt.Sprintf("webhook_queue_%s", clientID)

	_, err := r.ch.QueueDeclare(
		queueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %v", err)
	}

	err = r.ch.QueueBind(
		queueName,
		clientID, // routing key
		r.exchangeName,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue: %v", err)
	}

	return nil
}

// DeclareTopology ensures the core exchange, queue, and optional DLQ exist with the right bindings.
func DeclareTopology(ch *amqp.Channel, rabbitCfg config.RabbitMQConfig, logger *zap.Logger) (amqp.Queue, error) {
	// Declare primary exchange
	if err := ch.ExchangeDeclare(
		rabbitCfg.Exchange,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return amqp.Queue{}, fmt.Errorf("failed to declare exchange: %w", err)
	}

	queueArgs := amqp.Table{}
	if rabbitCfg.DLXName != "" {
		queueArgs["x-dead-letter-exchange"] = rabbitCfg.DLXName
		if rabbitCfg.DLXRoutingKey != "" {
			queueArgs["x-dead-letter-routing-key"] = rabbitCfg.DLXRoutingKey
		}
	}
	if rabbitCfg.MessageTTL > 0 {
		queueArgs["x-message-ttl"] = int32(rabbitCfg.MessageTTL)
	}

	q, err := ch.QueueDeclare(
		rabbitCfg.QueueName,
		true,
		false,
		false,
		false,
		queueArgs,
	)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("failed to declare queue: %w", err)
	}

	if err := ch.QueueBind(q.Name, "", rabbitCfg.Exchange, false, nil); err != nil {
		return amqp.Queue{}, fmt.Errorf("failed to bind queue: %w", err)
	}

	if rabbitCfg.DLXName != "" && rabbitCfg.DLXQueue != "" {
		if err := ch.ExchangeDeclare(
			rabbitCfg.DLXName,
			"fanout",
			true,
			false,
			false,
			false,
			nil,
		); err != nil {
			return amqp.Queue{}, fmt.Errorf("failed to declare DLX: %w", err)
		}

		dlq, err := ch.QueueDeclare(
			rabbitCfg.DLXQueue,
			true,
			false,
			false,
			false,
			nil,
		)
		if err != nil {
			return amqp.Queue{}, fmt.Errorf("failed to declare DLQ: %w", err)
		}

		bindingKey := rabbitCfg.DLXRoutingKey
		if bindingKey == "" {
			bindingKey = "dlq"
		}
		if err := ch.QueueBind(dlq.Name, bindingKey, rabbitCfg.DLXName, false, nil); err != nil {
			return amqp.Queue{}, fmt.Errorf("failed to bind DLQ: %w", err)
		}
		if logger != nil {
			logger.Info("DLQ configured",
				zap.String("exchange", rabbitCfg.DLXName),
				zap.String("queue", rabbitCfg.DLXQueue),
			)
		}
	}

	return q, nil
}
