package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-crud/entgo/mixin"
)

// SigningTemplate holds the schema definition for the SigningTemplate entity.
// Templates contain PDF files with placeholder fields for document signing.
type SigningTemplate struct {
	ent.Schema
}

// Annotations of the SigningTemplate.
func (SigningTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "paperless_signing_templates"},
		entsql.WithComments(true),
	}
}

// SigningTemplateFieldDef represents a field definition stored as JSON.
// Position fields are percentage-based coordinates set via the visual field builder.
type SigningTemplateFieldDef struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	Required       bool    `json:"required"`
	PageNumber     int     `json:"page_number"`
	XPercent       float64 `json:"x_percent"`
	YPercent       float64 `json:"y_percent"`
	WidthPercent   float64 `json:"width_percent"`
	HeightPercent  float64 `json:"height_percent"`
	PrefillStage   int     `json:"prefill_stage"`   // 0=not prefill, N=apply before recipient N
	RecipientIndex int     `json:"recipient_index"` // 0=all recipients, N=specific recipient
}

// Fields of the SigningTemplate.
func (SigningTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			NotEmpty().
			Unique().
			Comment("UUID primary key"),

		field.String("name").
			NotEmpty().
			MaxLen(255).
			Comment("Template display name"),

		field.String("description").
			Optional().
			MaxLen(4096).
			Comment("Template description"),

		field.String("file_key").
			NotEmpty().
			MaxLen(512).
			Comment("Storage key in RustFS/S3"),

		field.String("file_name").
			NotEmpty().
			MaxLen(255).
			Comment("Original file name"),

		field.Int64("file_size").
			Default(0).
			Comment("File size in bytes"),

		field.JSON("fields", []SigningTemplateFieldDef{}).
			Optional().
			Comment("Field definitions with positions from the visual builder"),
	}
}

// Edges of the SigningTemplate.
func (SigningTemplate) Edges() []ent.Edge {
	return nil
}

// Mixin of the SigningTemplate.
func (SigningTemplate) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.Time{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the SigningTemplate.
func (SigningTemplate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "name").Unique(),
		index.Fields("tenant_id"),
	}
}
