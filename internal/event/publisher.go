package event

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	"github.com/go-tangra/go-tangra-paperless/internal/conf"
)

// Publisher handles event publishing for paperless signing operations
type Publisher struct {
	log         *log.Helper
	redisClient *redis.Client
	config      *conf.EventConfig
	topicPrefix string
}

// NewPublisher creates a new event publisher
func NewPublisher(ctx *bootstrap.Context, redisClient *redis.Client) *Publisher {
	l := ctx.NewLoggerHelper("event/publisher/paperless-service")

	var eventConfig *conf.EventConfig
	var topicPrefix string = "paperless"

	customConfig, ok := ctx.GetCustomConfig("paperless")
	if ok {
		paperlessConfig, ok := customConfig.(*conf.Paperless)
		if ok && paperlessConfig != nil {
			eventConfig = paperlessConfig.GetEvents()
			if eventConfig != nil && eventConfig.GetTopicPrefix() != "" {
				topicPrefix = eventConfig.GetTopicPrefix()
			}
		}
	}

	return &Publisher{
		log:         l,
		redisClient: redisClient,
		config:      eventConfig,
		topicPrefix: topicPrefix,
	}
}

// IsEnabled returns true if event publishing is enabled
func (p *Publisher) IsEnabled() bool {
	if p.config == nil {
		return false
	}
	return p.config.GetEnabled()
}

// Publish publishes an event to the specified topic
func (p *Publisher) Publish(ctx context.Context, topic string, data any) error {
	if !p.IsEnabled() {
		return nil
	}

	if p.redisClient == nil {
		p.log.Warn("Redis client is nil, skipping event publish")
		return nil
	}

	event := SigningEvent{
		ID:        uuid.New().String(),
		Type:      topic,
		Source:    EventSource,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}

	// Extract tenant ID from the data if possible
	if tenantData, ok := data.(interface{ GetTenantID() uint32 }); ok {
		event.TenantID = tenantData.GetTenantID()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		p.log.Errorf("Failed to marshal event: %v", err)
		return err
	}

	fullTopic := p.topicPrefix + "." + topic
	if err := p.redisClient.Publish(ctx, fullTopic, payload).Err(); err != nil {
		p.log.Errorf("Failed to publish event to %s: %v", fullTopic, err)
		return err
	}

	p.log.Infof("Published event to %s: %s", fullTopic, event.ID)
	return nil
}

// PublishRequestCreated publishes a signing request created event
func (p *Publisher) PublishRequestCreated(ctx context.Context, data *SigningRequestCreatedData) error {
	return p.Publish(ctx, TopicSigningRequestCreated, data)
}

// PublishRecipientSigned publishes a signing recipient signed event
func (p *Publisher) PublishRecipientSigned(ctx context.Context, data *SigningRecipientSignedData) error {
	return p.Publish(ctx, TopicSigningRecipientSigned, data)
}

// PublishRequestCompleted publishes a signing request completed event
func (p *Publisher) PublishRequestCompleted(ctx context.Context, data *SigningRequestCompletedData) error {
	return p.Publish(ctx, TopicSigningRequestCompleted, data)
}

// PublishRequestCancelled publishes a signing request cancelled event
func (p *Publisher) PublishRequestCancelled(ctx context.Context, data *SigningRequestCancelledData) error {
	return p.Publish(ctx, TopicSigningRequestCancelled, data)
}
