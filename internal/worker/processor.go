package worker

import (
	"context"
	"encoding/json"
	"math"
	"math/rand/v2"
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
}

func NewWorker(channel *amqp.Channel, db *storage.MongoDB, logger *zap.Logger) *Worker {
	return &Worker{
		channel:    channel,
		db:         db,
		logger:     logger,
		maxRetries: 3,
		baseDelay:  10 * time.Second,
	}
}

func (w *Worker) Start(ctx context.Context, queueName string) error {
	// Log that the worker is starting consumption
	w.logger.Info("Worker started consuming messages", zap.String("queue", queueName))

	msgs, err := w.channel.Consume(
		queueName,
		"",    // consumer
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return err
	}

	go func() {
		for msg := range msgs {
			// Process message
			event := &models.WebhookEvent{
				Status:     string(models.EventStatusPending),
				ReceivedAt: time.Now().UTC(),
			}
			if err := json.Unmarshal(msg.Body, event); err != nil {
				w.logger.Error("Failed to unmarshal message",
					zap.Error(err),
					zap.String("body", string(msg.Body)))
				msg.Nack(false, false)
				continue
			}

			// Get metadata from headers
			// Log raw headers for debugging
			w.logger.Info("Processing message",
				zap.Any("headers", msg.Headers),
				zap.String("body", string(msg.Body)))

			// Extract metadata from headers
			if headers := msg.Headers; headers != nil {
				// Convert interface values to strings if present
				webhookID, _ := headers["webhook_id"].(string)
				webhookType, _ := headers["webhook_type"].(string)
				clientID, _ := headers["client_id"].(string)

				// Log extracted values
				w.logger.Info("Extracted metadata",
					zap.String("webhook_id", webhookID),
					zap.String("webhook_type", webhookType),
					zap.String("client_id", clientID))

				if webhookID != "" {
					event.WebhookID = webhookID
				}
				if webhookType != "" {
					event.WebhookType = webhookType
				}
				if clientID != "" {
					event.ClientID = clientID
				}
			}

			// Start processing timer
			start := time.Now()

			// Process the event
			if err := w.processEvent(ctx, event); err != nil {
				w.handleError(ctx, event, msg, err)
				continue
			}

			// Record metrics
			metrics.WebhookProcessed.WithLabelValues(event.ClientID, event.Event, "success").Inc()
			metrics.WebhookProcessingTime.WithLabelValues(event.ClientID, event.Event).Observe(time.Since(start).Seconds())

			// Success log for visibility
			w.logger.Info("Event processed and stored",
				zap.String("client_id", event.ClientID),
				zap.String("event", event.Event),
				zap.String("email", event.Email),
				zap.String("campaign_id", event.CampaignID),
				zap.String("webhook_id", event.WebhookID),
			)

			msg.Ack(false)
		}
	}()

	return nil
}

func (w *Worker) processEvent(ctx context.Context, event *models.WebhookEvent) error {
	// Store event in MongoDB
	if err := w.db.InsertEvent(ctx, event); err != nil {
		return err
	}

	// Update status
	return w.db.UpdateEventStatus(ctx, event, models.EventStatusProcessed)
}

func (w *Worker) handleError(ctx context.Context, event *models.WebhookEvent, msg amqp.Delivery, err error) {
	w.logger.Error("Failed to process event",
		zap.Error(err),
		zap.String("client_id", event.ClientID),
		zap.String("event", event.Event))

	event.RetryCount++
	metrics.WebhookRetries.WithLabelValues(event.ClientID, event.Event).Inc()

	if event.RetryCount >= w.maxRetries {
		// Max retries reached, mark as failed
		if err := w.db.UpdateEventStatus(ctx, event, models.EventStatusFailed); err != nil {
			w.logger.Error("Failed to update event status", zap.Error(err))
		}
		msg.Ack(false)
		return
	}

	// Calculate exponential backoff delay
	delay := w.calculateBackoff(event.RetryCount)

	// Update status to retrying
	if err := w.db.UpdateEventStatus(ctx, event, models.EventStatusRetrying); err != nil {
		w.logger.Error("Failed to update event status", zap.Error(err))
	}

	// Requeue with delay
	time.Sleep(delay)
	msg.Nack(false, true)
}

func (w *Worker) calculateBackoff(retryCount int) time.Duration {
	// Exponential backoff with jitter
	backoff := float64(w.baseDelay) * math.Pow(2, float64(retryCount-1))
	jitter := (rand.Float64()*0.5 + 0.5) // 50% jitter
	return time.Duration(backoff * jitter)
}
