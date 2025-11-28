package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"webhook-processor/internal/models"
	"webhook-processor/internal/storage"
	"webhook-processor/pkg/metrics"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Worker struct {
	channel    *amqp.Channel
	db         *storage.MongoDB
	logger     *zap.Logger
	maxRetries int
	baseDelay  time.Duration
	concurrency int
	consumerTag string
	wg          sync.WaitGroup
}

func NewWorker(channel *amqp.Channel, db *storage.MongoDB, logger *zap.Logger, concurrency int) *Worker {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Worker{
		channel:    channel,
		db:         db,
		logger:     logger,
		maxRetries: 3,
		baseDelay:  5 * time.Second,
		concurrency: concurrency,
	}
}

func (w *Worker) Start(ctx context.Context, queueName string) error {
	w.consumerTag = fmt.Sprintf("webhook-worker-%d", time.Now().UnixNano())
	if err := w.channel.Qos(w.concurrency*2, 0, false); err != nil {
		return err
	}

	msgs, err := w.channel.Consume(
		queueName,
		w.consumerTag,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	w.logger.Info("Worker consuming queue",
		zap.String("queue", queueName),
		zap.Int("concurrency", w.concurrency))

	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go func(id int) {
			defer w.wg.Done()
			w.consumeLoop(ctx, msgs)
		}(i)
	}

	go func() {
		<-ctx.Done()
		w.logger.Info("Stopping worker consumers")
		_ = w.channel.Cancel(w.consumerTag, false)
	}()

	return nil
}

// Wait blocks until all worker goroutines finish processing.
func (w *Worker) Wait() {
	w.wg.Wait()
	w.logger.Info("All worker consumers stopped")
}

func (w *Worker) processEvent(ctx context.Context, event *models.WebhookEvent) error {
	// Store event in MongoDB
	if err := w.db.InsertEvent(ctx, event); err != nil {
		return err
	}

	// Update status
	return w.db.UpdateEventStatus(ctx, event, models.EventStatusProcessed)
}

func (w *Worker) consumeLoop(ctx context.Context, msgs <-chan amqp.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			w.processDelivery(ctx, msg)
		}
	}
}

func (w *Worker) processDelivery(ctx context.Context, msg amqp.Delivery) {
	event := &models.WebhookEvent{
		Status:     string(models.EventStatusPending),
		ReceivedAt: time.Now().UTC(),
	}
	if err := json.Unmarshal(msg.Body, event); err != nil {
		w.logger.Error("Failed to unmarshal message",
			zap.Error(err),
			zap.String("body", string(msg.Body)))
		msg.Nack(false, false)
		return
	}

	w.applyHeaders(event, msg.Headers)

	for attempt := 1; attempt <= w.maxRetries; attempt++ {
		start := time.Now()
		if err := w.processEvent(ctx, event); err == nil {
			metrics.WebhookProcessed.WithLabelValues(event.ClientID, event.Event, "success").Inc()
			metrics.WebhookProcessingTime.WithLabelValues(event.ClientID, event.Event).Observe(time.Since(start).Seconds())
			w.logger.Debug("Event processed",
				zap.String("client_id", event.ClientID),
				zap.String("event", event.Event),
				zap.String("webhook_id", event.WebhookID))
			msg.Ack(false)
			return
		} else {
			event.RetryCount = attempt
			metrics.WebhookRetries.WithLabelValues(event.ClientID, event.Event).Inc()
			w.logger.Warn("Retrying event",
				zap.Error(err),
				zap.Int("attempt", attempt),
				zap.String("client_id", event.ClientID),
				zap.String("event", event.Event))

			if attempt < w.maxRetries {
				delay := w.calculateBackoff(attempt)
				if err := w.db.UpdateEventStatus(ctx, event, models.EventStatusRetrying); err != nil {
					w.logger.Error("Failed to update event status", zap.Error(err))
				}
				select {
				case <-ctx.Done():
					msg.Nack(false, true)
					return
				case <-time.After(delay):
				}
				continue
			}
		}
	}

	if err := w.db.UpdateEventStatus(ctx, event, models.EventStatusFailed); err != nil {
		w.logger.Error("Failed to mark event as failed", zap.Error(err))
	}
	metrics.WebhookProcessed.WithLabelValues(event.ClientID, event.Event, "failed").Inc()
	msg.Ack(false)
}

func (w *Worker) applyHeaders(event *models.WebhookEvent, headers amqp.Table) {
	if headers == nil {
		return
	}
	if webhookID, _ := headers["webhook_id"].(string); webhookID != "" {
		event.WebhookID = webhookID
	}
	if webhookType, _ := headers["webhook_type"].(string); webhookType != "" {
		event.WebhookType = webhookType
	}
	if clientID, _ := headers["client_id"].(string); clientID != "" {
		event.ClientID = clientID
	}
}

func (w *Worker) calculateBackoff(retryCount int) time.Duration {
	// Exponential backoff with jitter
	backoff := float64(w.baseDelay) * math.Pow(2, float64(retryCount-1))
	jitter := (rand.Float64()*0.5 + 0.5) // 50% jitter
	return time.Duration(backoff * jitter)
}
