package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"webhook-processor/internal/models"
	"webhook-processor/pkg/metrics"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Publisher interface {
	Publish(event models.WebhookEvent) error
	Close() error
}

type RabbitMQ struct {
	conn         *amqp.Connection
	ch           *amqp.Channel
	exchangeName string
	logger       *zap.Logger
	queueName    string
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

func NewRabbitMQ(url, exchangeName, queueName string, logger *zap.Logger) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %v", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %v", err)
	}

	// Declare exchange
	err = ch.ExchangeDeclare(
		exchangeName,
		"direct",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %v", err)
	}

	// Declare queue
	q, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %v", err)
	}

	// Bind queue to exchange
	err = ch.QueueBind(
		q.Name,       // queue name
		"",           // routing key
		exchangeName, // exchange
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to bind queue: %v", err)
	}

	return &RabbitMQ{
		conn:         conn,
		ch:           ch,
		exchangeName: exchangeName,
		logger:       logger,
		queueName:    queueName,
	}, nil
}

func (r *RabbitMQ) Publish(event models.WebhookEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %v", err)
	}

	// Add headers with the client ID for the worker to use
	headers := make(amqp.Table)
	headers["webhook_id"] = event.WebhookID
	headers["webhook_type"] = event.WebhookType
	headers["client_id"] = event.ClientID

	// Publish to all queues bound to this exchange
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

	if err != nil {
		return fmt.Errorf("failed to publish message: %v", err)
	}

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
