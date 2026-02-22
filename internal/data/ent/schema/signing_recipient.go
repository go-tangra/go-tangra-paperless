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

// SigningRecipient holds the schema definition for the SigningRecipient entity.
// Represents a single recipient in a signing request with their signing status.
type SigningRecipient struct {
	ent.Schema
}

// Annotations of the SigningRecipient.
func (SigningRecipient) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "paperless_signing_recipients"},
		entsql.WithComments(true),
	}
}

// Fields of the SigningRecipient.
func (SigningRecipient) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			NotEmpty().
			Unique().
			Comment("UUID primary key"),

		field.String("signing_request_id").
			NotEmpty().
			MaxLen(36).
			Comment("Reference to signing request"),

		field.String("email").
			NotEmpty().
			MaxLen(255).
			Comment("Recipient email address"),

		field.String("name").
			NotEmpty().
			MaxLen(255).
			Comment("Recipient display name"),

		field.Int32("signing_order").
			Default(1).
			Comment("Order in which recipients sign (1-based)"),

		field.Enum("status").
			Values(
				"SIGNING_RECIPIENT_STATUS_UNSPECIFIED",
				"SIGNING_RECIPIENT_STATUS_WAITING",
				"SIGNING_RECIPIENT_STATUS_PENDING",
				"SIGNING_RECIPIENT_STATUS_COMPLETED",
				"SIGNING_RECIPIENT_STATUS_DECLINED",
			).
			Default("SIGNING_RECIPIENT_STATUS_WAITING").
			Comment("Recipient signing status"),

		field.String("token").
			Unique().
			MaxLen(64).
			Comment("Unique signing token (64-char hex)"),

		field.Time("signed_at").
			Optional().
			Nillable().
			Comment("Timestamp when recipient signed"),

		field.String("signed_ip").
			Optional().
			MaxLen(45).
			Comment("IP address of signer"),

		field.Bytes("signature_data").
			Optional().
			Comment("Signature image data (PNG)"),
	}
}

// Edges of the SigningRecipient.
func (SigningRecipient) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("signing_request", SigningRequest.Type).
			Ref("recipients").
			Field("signing_request_id").
			Unique().
			Required().
			Comment("Parent signing request"),
	}
}

// Mixin of the SigningRecipient.
func (SigningRecipient) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

// Indexes of the SigningRecipient.
func (SigningRecipient) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token").Unique(),
		index.Fields("signing_request_id", "signing_order"),
		index.Fields("signing_request_id"),
	}
}
