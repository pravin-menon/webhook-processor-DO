package router

import (
	"os"
	"strings"
	"webhook-processor/api/handlers"
	"webhook-processor/api/middleware"
	"webhook-processor/config"
	"webhook-processor/internal/mapping"
	"webhook-processor/internal/queue"
	"webhook-processor/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// WebhookHandler interface for both standard and debug handlers
type WebhookHandler interface {
	HandleWebhook(c *gin.Context)
}

func Setup(logger *logger.Logger, publisher queue.Publisher, cfg *config.Config) *gin.Engine {
	router := gin.Default()

	// Initialize webhook mapping service
	webhookMapper := mapping.NewWebhookMappingService(logger.Desugar())
	if webhookMapper == nil {
		logger.Desugar().Error("Failed to initialize webhook mapping service")
	} else {
		// Load webhook mappings from environment
		if err := webhookMapper.LoadMappingFromEnvironment(); err != nil {
			logger.Desugar().Error("Failed to load webhook mappings", zap.Error(err))
			// Continue without mappings - will fall back to domain-based identification
		} else {
			logger.Desugar().Info("Successfully loaded webhook mappings from environment")
		}
	}

	// Initialize security middleware
	security := middleware.NewSecurityMiddleware(
		logger.Desugar(),
		cfg.Security.APIKeys,
		cfg.Security.APIKeyHeader,
	)

	// Apply global middleware
	router.Use(security.CORS())

	// Health check endpoint (no authentication required)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Metrics endpoint for Prometheus (no authentication required)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Initialize webhook handler (debug or production based on environment)
	var webhookHandler WebhookHandler
	if os.Getenv("WEBHOOK_DEBUG") == "true" {
		logger.Desugar().Info("Initializing DEBUG webhook handler")
		webhookHandler = handlers.NewDebugMailerCloudWebhookHandler(logger.Desugar(), publisher, webhookMapper)
	} else {
		logger.Desugar().Info("Initializing PRODUCTION webhook handler")
		webhookHandler = handlers.NewMailerCloudWebhookHandler(logger.Desugar(), publisher, webhookMapper, cfg.Webhook)
	}

	// Public webhook validation endpoint for MailerCloud (no authentication required)
	router.GET("/webhook", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "Webhook endpoint is ready",
			"service": "MailerCloud Webhook Processor",
		})
	})

	// Webhook POST endpoint with conditional authentication
	router.POST("/webhook", func(c *gin.Context) {
		// Check if this is a MailerCloud validation request
		webhookId := c.GetHeader("Webhook-Id")
		webhookType := c.GetHeader("Webhook-Type")
		userAgent := c.GetHeader("User-Agent")
		contentType := c.GetHeader("Content-Type")

		// Log incoming request for debugging
		logger.Desugar().Info("Incoming webhook POST request",
			zap.String("webhook_id", webhookId),
			zap.String("webhook_type", webhookType),
			zap.String("user_agent", userAgent),
			zap.String("content_type", contentType),
			zap.String("method", c.Request.Method))

		// MailerCloud validation scenarios (do NOT read/consume body here):
		// 1. Webhook-Id header with "WebhookID" value (classic validation)
		// 2. User-Agent equals/contains "MailerCloud" (test requests)
		isMailerCloudValidation := webhookId == "WebhookID" || userAgent == "MailerCloud" ||
			// Some clients send UA variations like "MailerCloud/<version>"
			(strings.Contains(userAgent, "MailerCloud"))

		if isMailerCloudValidation {
			// This is MailerCloud validation - return success
			logger.Desugar().Info("Handling MailerCloud validation request",
				zap.String("webhook_id", webhookId),
				zap.String("user_agent", userAgent))
			c.JSON(200, gin.H{
				"status":  "ok",
				"message": "Webhook validation successful",
				"service": "MailerCloud Webhook Processor",
				"success": true,
			})
			return
		}

		// For MailerCloud webhooks (real ones have Webhook-Id but not "WebhookID")
		// MailerCloud doesn't send API keys - they authenticate via URL validation
		if webhookId != "" && webhookId != "WebhookID" {
			// This is a real MailerCloud webhook - process without API key requirement
			logger.Desugar().Info("Processing MailerCloud webhook",
				zap.String("webhook_id", webhookId),
				zap.String("webhook_type", webhookType))
			webhookHandler.HandleWebhook(c)
			return
		}

		// For other webhooks (non-MailerCloud), require authentication
		apiKey := c.GetHeader(cfg.Security.APIKeyHeader)
		if apiKey == "" {
			c.JSON(401, gin.H{"error": "Missing API key"})
			return
		}

		// Validate API key
		var validKey bool
		for _, key := range cfg.Security.APIKeys {
			if key == apiKey {
				validKey = true
				break
			}
		}

		if !validKey {
			c.JSON(401, gin.H{"error": "Invalid API key"})
			return
		}

		// Process authenticated webhook
		webhookHandler.HandleWebhook(c)
	})

	logger.Desugar().Info("Router configured with security middleware",
		zap.String("api_key_header", cfg.Security.APIKeyHeader),
		zap.Int("configured_clients", len(cfg.Security.APIKeys)),
	)

	return router
}
