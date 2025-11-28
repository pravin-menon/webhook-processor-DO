package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"webhook-processor/config"
	"webhook-processor/internal/mapping"
	"webhook-processor/internal/models"
	"webhook-processor/internal/queue"
	"webhook-processor/pkg/metrics"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type MailerCloudWebhookHandler struct {
	logger        *zap.Logger
	publisher     queue.Publisher
	rateLimiter   *RateLimiter
	webhookMapper *mapping.WebhookMappingService
	webhookCfg    config.WebhookConfig
	failureTracker *FailureTracker
}

func NewMailerCloudWebhookHandler(logger *zap.Logger, publisher queue.Publisher, webhookMapper *mapping.WebhookMappingService, webhookCfg config.WebhookConfig) *MailerCloudWebhookHandler {
	if webhookCfg.IngressRetryCount <= 0 {
		webhookCfg.IngressRetryCount = 1
	}
	if webhookCfg.IngressRetryDelay <= 0 {
		webhookCfg.IngressRetryDelay = 5 * time.Second
	}
	if webhookCfg.MaxConsecutiveFailures <= 0 {
		webhookCfg.MaxConsecutiveFailures = 20
	}
	return &MailerCloudWebhookHandler{
		logger:        logger,
		publisher:     publisher,
		rateLimiter:   NewRateLimiter(),
		webhookMapper: webhookMapper,
		webhookCfg:    webhookCfg,
		failureTracker: NewFailureTracker(),
	}
}

func (h *MailerCloudWebhookHandler) HandleWebhook(c *gin.Context) {
	// Start timing for metrics
	start := time.Now()
	var clientID string

	// Handle GET requests for URL validation
	if c.Request.Method == "GET" {
		h.logger.Info("Handling GET request for webhook validation")
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "Webhook endpoint is valid",
			"service": "MailerCloud Webhook Processor",
			"success": true,
		})
		return
	}

	// For MailerCloud webhooks, parse the request body
	var data map[string]interface{}
	if err := c.ShouldBindJSON(&data); err != nil {
		h.logger.Error("Failed to parse webhook payload",
			zap.Error(err),
			zap.String("content_type", c.GetHeader("Content-Type")),
			zap.String("user_agent", c.GetHeader("User-Agent")),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
		return
	}

	// Log request details at debug level to avoid flooding production logs
	h.logger.Debug("Received webhook request",
		zap.String("method", c.Request.Method),
		zap.String("content-type", c.GetHeader("Content-Type")),
		zap.String("user-agent", c.GetHeader("User-Agent")),
		zap.String("webhook-id", c.GetHeader("Webhook-Id")),
		zap.String("webhook-type", c.GetHeader("Webhook-Type")),
		zap.Any("payload", data),
	)

	// Handle various MailerCloud test/validation scenarios
	userAgent := c.GetHeader("User-Agent")
	webhookId := c.GetHeader("Webhook-Id")

	// Validation scenarios:
	// 1. User-Agent is "MailerCloud" (classic test requests)
	// 2. Webhook-Id is "WebhookID" (URL validation)
	// 3. Empty or test payload
	isValidationRequest := false

	if userAgent == "MailerCloud" || webhookId == "WebhookID" {
		isValidationRequest = true
	}

	// Check for test payload patterns
	if len(data) == 0 || (len(data) == 1 && data["test"] != nil) {
		isValidationRequest = true
	}

	// Also check for common test event patterns
	if event, ok := data["event"].(string); ok {
		if event == "test" || event == "validation" || event == "ping" {
			isValidationRequest = true
		}
	}

	if isValidationRequest {
		h.logger.Info("Handling MailerCloud validation/test request",
			zap.String("user_agent", userAgent),
			zap.String("webhook_id", webhookId),
			zap.Any("payload", data))
		metrics.WebhookReceived.WithLabelValues("test", "validation").Inc()
		c.JSON(http.StatusOK, gin.H{
			"message": "Webhook URL verified",
			"success": true,
			"status":  "ok",
			"service": "MailerCloud Webhook Processor",
		})
		return
	}

	// Extract client ID using the webhook mapping service
	clientID = h.extractClientID(c, data)

	// Check rate limits for the identified client
	if !h.rateLimiter.AllowRequest(clientID) {
		metrics.RateLimitExceeded.WithLabelValues(clientID, "requests").Inc()
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
		return
	}

	// Create webhook event from request body
	event := models.WebhookEvent{
		WebhookID:   h.generateWebhookID(data),
		WebhookType: "email_event",
		ClientID:    clientID,
		ReceivedAt:  time.Now().UTC(),
		Status:      string(models.EventStatusPending),
	}

	// Extract fields from the payload with type assertions and error handling
	if val, ok := data["event"].(string); ok {
		event.Event = val
	}

	// Campaign name variations
	if val, ok := data["campaign_name"].(string); ok {
		event.CampaignName = val
	} else if val, ok := data["campaign name"].(string); ok {
		event.CampaignName = val
	}

	// Campaign ID variations
	if val, ok := data["campaign_id"].(string); ok {
		event.CampaignID = val
	} else if val, ok := data["camp_id"].(string); ok {
		event.CampaignID = val
	}

	// Tag name variations
	if val, ok := data["tag_name"].(string); ok {
		event.TagName = val
	} else if val, ok := data["tag"].(string); ok {
		event.TagName = val
	}

	if val, ok := data["date_event"].(string); ok {
		event.DateEvent = val
	}
	if val, ok := data["ts"].(float64); ok {
		event.Timestamp = int64(val)
	}
	if val, ok := data["ts_event"].(float64); ok {
		event.TimestampEvent = int64(val)
	}
	if val, ok := data["email"].(string); ok {
		event.Email = val
	}

	// URL field variations (for click events)
	if val, ok := data["URL"].(string); ok {
		event.URL = val
	} else if val, ok := data["url"].(string); ok {
		event.URL = val
	} else if val, ok := data["click_url"].(string); ok {
		event.URL = val
	}

	// Reason field (for bounce, spam, campaign_error events)
	if val, ok := data["reason"].(string); ok {
		event.Reason = val
	}

	// Ensure variables are set for metrics (after all parsing)
	clientID = event.ClientID

	// Handle list_id which can be string, number, or array (for unsubscribe events)
	if val, exists := data["list_id"]; exists {
		event.ListID = val
	}

	// Handle emails array
	if val, ok := data["emails"].([]interface{}); ok {
		emails := make([]string, 0, len(val))
		for _, email := range val {
			if emailStr, ok := email.(string); ok {
				emails = append(emails, emailStr)
			}
		}
		event.Emails = emails
	}

	// Record the received event metric
	metrics.WebhookReceived.WithLabelValues(event.ClientID, event.Event).Inc()

	// Send the event to the message queue with ingress-level retries
	if err := h.publishWithRetry(c.Request.Context(), event); err != nil {
		metrics.WebhookProcessed.WithLabelValues(event.ClientID, event.Event, "failed").Inc()
		failureCount := h.failureTracker.RecordFailure(h.failureKey(event))
		metrics.WebhookIngressConsecutiveFailures.WithLabelValues(event.ClientID, event.WebhookID).Set(float64(failureCount))
		if failureCount >= h.webhookCfg.MaxConsecutiveFailures {
			h.logger.Error("Ingress consecutive failure limit reached",
				zap.Int("failures", failureCount),
				zap.Int("limit", h.webhookCfg.MaxConsecutiveFailures),
				zap.String("webhook_id", event.WebhookID),
				zap.String("client_id", event.ClientID))
		} else if failureCount >= h.webhookCfg.MaxConsecutiveFailures/2 {
			h.logger.Warn("Ingress consecutive failures approaching limit",
				zap.Int("failures", failureCount),
				zap.Int("limit", h.webhookCfg.MaxConsecutiveFailures),
				zap.String("webhook_id", event.WebhookID),
				zap.String("client_id", event.ClientID))
		}

		// Record processing time metric for failed requests too
		if event.ClientID != "" && event.Event != "" {
			duration := time.Since(start).Seconds()
			metrics.WebhookProcessingTime.WithLabelValues(event.ClientID, event.Event).Observe(duration)
		}

		h.logger.Error("Failed to publish event",
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process event"})
		return
	}

	metrics.WebhookProcessed.WithLabelValues(event.ClientID, event.Event, "success").Inc()
	h.failureTracker.RecordSuccess(h.failureKey(event))
	metrics.WebhookIngressConsecutiveFailures.WithLabelValues(event.ClientID, event.WebhookID).Set(0)

	// Record processing time metric
	if event.ClientID != "" && event.Event != "" {
		duration := time.Since(start).Seconds()
		metrics.WebhookProcessingTime.WithLabelValues(event.ClientID, event.Event).Observe(duration)
		h.logger.Info("Recorded processing time metric",
			zap.String("client_id", event.ClientID),
			zap.String("event", event.Event),
			zap.Float64("duration_seconds", duration))
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Event accepted",
		"webhook_id": event.WebhookID,
		"client_id":  event.ClientID,
	})
}

func (h *MailerCloudWebhookHandler) publishWithRetry(ctx context.Context, event models.WebhookEvent) error {
	attempts := h.webhookCfg.IngressRetryCount
	if attempts < 1 {
		attempts = 1
	}
	delay := h.webhookCfg.IngressRetryDelay
	if delay <= 0 {
		delay = 5 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := h.publisher.Publish(event); err == nil {
			if attempt > 1 {
				h.logger.Info("Publish succeeded after retry",
					zap.Int("attempt", attempt),
					zap.String("webhook_id", event.WebhookID),
					zap.String("client_id", event.ClientID))
			}
			return nil
		} else {
			lastErr = err
			h.logger.Warn("Failed to publish event",
				zap.Error(err),
				zap.Int("attempt", attempt),
				zap.String("webhook_id", event.WebhookID),
				zap.String("client_id", event.ClientID))

			if attempt < attempts {
				metrics.WebhookIngressRetryCount.WithLabelValues(event.ClientID, event.WebhookID).Inc()
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("failed to publish event after %d attempts", h.webhookCfg.IngressRetryCount)
	}
	return lastErr
}

func (h *MailerCloudWebhookHandler) failureKey(event models.WebhookEvent) string {
	return fmt.Sprintf("%s:%s", event.ClientID, event.WebhookID)
}

// extractClientID identifies the client using webhook ID mapping
func (h *MailerCloudWebhookHandler) extractClientID(c *gin.Context, data map[string]interface{}) string {
	// Primary Strategy: Use Webhook-Id header to lookup client via mapping service
	webhookID := c.GetHeader("Webhook-Id")
	if webhookID != "" && h.webhookMapper != nil {
		h.logger.Info("Attempting to lookup client via webhook ID", zap.String("webhook_id", webhookID))

		if clientID, found := h.webhookMapper.GetClientForWebhook(webhookID); found {
			h.logger.Info("Successfully mapped webhook ID to client",
				zap.String("webhook_id", webhookID),
				zap.String("client_id", clientID))
			return clientID
		}

		h.logger.Warn("Webhook ID not found in mapping, falling back to webhook ID",
			zap.String("webhook_id", webhookID))
	}

	// Fallback: Use webhook ID as client identifier if available
	if webhookID != "" {
		return webhookID
	}

	// Final fallback: Unknown client
	return "unknown"
}

// generateWebhookID creates a unique ID for the webhook event
func (h *MailerCloudWebhookHandler) generateWebhookID(data map[string]interface{}) string {
	// Strategy 1: Use existing webhook/message ID if available
	idFields := []string{"webhook_id", "message_id", "event_id", "delivery_id", "tracking_id"}
	for _, field := range idFields {
		if val, ok := data[field].(string); ok && val != "" {
			return val
		}
	}

	// Strategy 2: Generate based on combination of fields for uniqueness
	var components []string

	if val, ok := data["campaign_id"].(string); ok && val != "" {
		components = append(components, val)
	}
	if val, ok := data["email"].(string); ok && val != "" {
		components = append(components, val)
	}
	if val, ok := data["ts"].(float64); ok {
		components = append(components, fmt.Sprintf("%.0f", val))
	}
	if val, ok := data["event"].(string); ok && val != "" {
		components = append(components, val)
	}

	if len(components) > 0 {
		return fmt.Sprintf("mc_%x", components)
	}

	// Strategy 3: Fallback to timestamp-based ID
	return fmt.Sprintf("mc_%d", time.Now().UnixNano())
}
