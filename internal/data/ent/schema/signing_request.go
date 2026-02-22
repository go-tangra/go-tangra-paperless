package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-crud/entgo/mixin"
)

// SigningRequest holds the schema definition for the SigningRequest entity.
// Represents a document signing request with multiple recipients.
type SigningRequest struct {
	ent.Schema
}

// Annotations of the SigningRequest.
func (SigningRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "paperless_signing_requests"},
		entsql.WithComments(true),
	}
}

// SigningFieldValueDef represents a filled field value stored as JSON.
type SigningFieldValueDef struct {
	FieldID   string `json:"field_id"`
	FieldName string `json:"field_name"`
	Value     string `json:"value"`
	FilledBy  string `json:"filled_by"`
}

// Fields of the SigningRequest.
func (SigningRequest) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			NotEmpty().
			Unique().
			Comment("UUID primary key"),

		field.String("template_id").
			NotEmpty().
			MaxLen(36).
			Comment("Reference to signing template"),

		field.String("name").
			NotEmpty().
			MaxLen(255).
			Comment("Request display name"),

		field.Enum("status").
			Values(
				"SIGNING_REQUEST_STATUS_UNSPECIFIED",
				"SIGNING_REQUEST_STATUS_DRAFT",
				"SIGNING_REQUEST_STATUS_PENDING",
				"SIGNING_REQUEST_STATUS_COMPLETED",
				"SIGNING_REQUEST_STATUS_CANCELLED",
				"SIGNING_REQUEST_STATUS_EXPIRED",
			).
			Default("SIGNING_REQUEST_STATUS_DRAFT").
			Comment("Request status"),

		field.String("original_file_key").
			Optional().
			MaxLen(512).
			Comment("Storage key for the original PDF copy"),

		field.String("signed_file_key").
			Optional().
			MaxLen(512).
			Comment("Storage key for the signed/completed PDF"),

		field.JSON("field_values", []SigningFieldValueDef{}).
			Optional().
			Comment("Filled field values from all signers"),

		field.String("message").
			Optional().
			MaxLen(4096).
			Comment("Custom message to recipients"),

		field.Time("expires_at").
			Optional().
			Nillable().
			Comment("Expiration time for the signing request"),
	}
}

// Edges of the SigningRequest.
func (SigningRequest) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("recipients", SigningRecipient.Type).
			Comment("Recipients of this signing request"),
	}
}

// Mixin of the SigningRequest.
func (SigningRequest) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.Time{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the SigningRequest.
func (SigningRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id"),
		index.Fields("status"),
		index.Fields("template_id"),
		index.Fields("tenant_id", "status"),
	}
}
