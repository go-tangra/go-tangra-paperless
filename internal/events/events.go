// Package events publishes document processing-status events to the shared
// platform event bus so the gateway SSE hub relays them to the browser. The
// module only publishes (it consumes no external events); payloads carry no
// document content or credentials.
package events

import (
	"context"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/stream"
)

// Event types published to platform:events:<tenant>.
const (
	Processing = "document.processing"
	Completed  = "document.completed"
	Failed     = "document.failed"
)

// Publisher emits a realtime event to all of a tenant's subscribers.
type Publisher interface {
	Publish(ctx context.Context, tenantID, eventType string, payload any)
}

// HubPublisher publishes through the stream hub (nil hub is a no-op).
type HubPublisher struct{ Hub *stream.Hub }

// Publish broadcasts eventType to every subscriber of tenantID.
func (p HubPublisher) Publish(ctx context.Context, tenantID, eventType string, payload any) {
	if p.Hub == nil {
		return
	}
	_, _ = p.Hub.PublishID(ctx, tenantID, nil, true, eventType, payload, false)
}

// DocumentPayload is the (content-free) payload for a processing event.
func DocumentPayload(documentID, processingStatus, name string) map[string]any {
	return map[string]any{"document_id": documentID, "processing_status": processingStatus, "name": name}
}
