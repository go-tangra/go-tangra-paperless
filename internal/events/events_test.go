package events

import (
	"context"
	"testing"

	"github.com/go-freya/freya/services/paperless/internal/stream"
)

const tenant = "0190f7c2-6a3e-7c1a-9b2e-2f6f9d1b4c55"

func TestEventTypeConsts(t *testing.T) {
	if Processing != "document.processing" || Completed != "document.completed" || Failed != "document.failed" {
		t.Fatalf("unexpected event type consts: %q %q %q", Processing, Completed, Failed)
	}
}

func TestDocumentPayloadShape(t *testing.T) {
	p := DocumentPayload("doc-1", Completed, "invoice.pdf")
	if p["document_id"] != "doc-1" || p["processing_status"] != Completed || p["name"] != "invoice.pdf" {
		t.Fatalf("payload shape: %+v", p)
	}
	// The payload is content-free: only the three declared keys are present.
	if len(p) != 3 {
		t.Fatalf("payload should carry exactly 3 keys, got %d: %+v", len(p), p)
	}
}

func TestHubPublisherNilHubIsNoop(t *testing.T) {
	// A nil hub must be a safe no-op (does not panic).
	var p HubPublisher
	p.Publish(context.Background(), tenant, Processing, DocumentPayload("d", Processing, "n"))

	// Interface satisfaction.
	var _ Publisher = HubPublisher{}
}

func TestHubPublisherDeliversToStream(t *testing.T) {
	mem := stream.NewMemory()
	hub := stream.NewHub(mem, stream.Config{}, nil)
	defer hub.Close()

	p := HubPublisher{Hub: hub}
	ctx := context.Background()
	p.Publish(ctx, tenant, Completed, DocumentPayload("doc-9", Completed, "report.pdf"))

	// The event landed in the tenant's stream as a tenant-wide (all) broadcast.
	key := stream.Key(tenant)
	if mem.Len(key) != 1 {
		t.Fatalf("expected one stream entry, got %d", mem.Len(key))
	}
	entries, err := mem.XRange(ctx, key, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	e := entries[0]
	if e.Fields["type"] != Completed {
		t.Fatalf("event type in stream: %q", e.Fields["type"])
	}
	if e.Fields["to"] != "*" {
		t.Fatalf("expected broadcast target '*', got %q", e.Fields["to"])
	}
	if e.Fields["data"] == "" || e.Fields["data"] == "{}" {
		t.Fatalf("expected a JSON payload, got %q", e.Fields["data"])
	}
}
