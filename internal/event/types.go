package event

import (
	"time"
)

// Signing event topics - these are the topic suffixes (prefix is added by publisher)
const (
	TopicSigningRequestCreated   = "signing.request.created"
	TopicSigningRecipientSigned  = "signing.recipient.signed"
	TopicSigningRequestCompleted = "signing.request.completed"
	TopicSigningRequestCancelled = "signing.request.cancelled"
)

// Event source identifier
const EventSource = "paperless-service"

// SigningEvent is the base event envelope for all paperless signing events
type SigningEvent struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Source    string            `json:"source"`
	Timestamp time.Time         `json:"timestamp"`
	TenantID  uint32            `json:"tenant_id"`
	Data      interface{}       `json:"data"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SigningRequestCreatedData is published when a new signing request is created
type SigningRequestCreatedData struct {
	RequestID      string `json:"request_id"`
	TemplateID     string `json:"template_id"`
	TemplateName   string `json:"template_name"`
	RecipientCount int    `json:"recipient_count"`
	TenantID       uint32 `json:"tenant_id"`
}

// SigningRecipientSignedData is published when a recipient signs
type SigningRecipientSignedData struct {
	RequestID      string `json:"request_id"`
	RecipientEmail string `json:"recipient_email"`
	RecipientName  string `json:"recipient_name"`
	SigningOrder   int32  `json:"signing_order"`
	RemainingCount int    `json:"remaining_count"`
	TenantID       uint32 `json:"tenant_id"`
}

// SigningRequestCompletedData is published when the last recipient signs
type SigningRequestCompletedData struct {
	RequestID    string `json:"request_id"`
	TemplateID   string `json:"template_id"`
	TemplateName string `json:"template_name"`
	SignedFileKey string `json:"signed_file_key"`
	TenantID     uint32 `json:"tenant_id"`
}

// SigningRequestCancelledData is published when a signing request is cancelled
type SigningRequestCancelledData struct {
	RequestID string `json:"request_id"`
	TenantID  uint32 `json:"tenant_id"`
}

// GetTenantID implements the tenant ID extraction interface used by the publisher
func (d *SigningRequestCreatedData) GetTenantID() uint32  { return d.TenantID }
func (d *SigningRecipientSignedData) GetTenantID() uint32 { return d.TenantID }
func (d *SigningRequestCompletedData) GetTenantID() uint32 { return d.TenantID }
func (d *SigningRequestCancelledData) GetTenantID() uint32 { return d.TenantID }
