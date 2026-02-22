package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/timestamppb"

	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/go-tangra/go-tangra-paperless/internal/data/ent"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/schema"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/signingtemplate"

	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
)

type SigningTemplateRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *log.Helper
}

func NewSigningTemplateRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *SigningTemplateRepo {
	return &SigningTemplateRepo{
		log:       ctx.NewLoggerHelper("paperless/signing-template/repo"),
		entClient: entClient,
	}
}

// Create creates a new signing template
func (r *SigningTemplateRepo) Create(ctx context.Context, tenantID uint32, name, description, fileKey, fileName string, fileSize int64, fields []schema.SigningTemplateFieldDef, createdBy *uint32) (*ent.SigningTemplate, error) {
	id := uuid.New().String()

	builder := r.entClient.Client().SigningTemplate.Create().
		SetID(id).
		SetTenantID(tenantID).
		SetName(name).
		SetFileKey(fileKey).
		SetFileName(fileName).
		SetFileSize(fileSize).
		SetCreateTime(time.Now())

	if description != "" {
		builder.SetDescription(description)
	}
	if fields != nil {
		builder.SetFields(fields)
	}
	if createdBy != nil {
		builder.SetCreateBy(*createdBy)
	}

	entity, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, paperlessV1.ErrorConflict("signing template with this name already exists")
		}
		r.log.Errorf("create signing template failed: %s", err.Error())
		return nil, paperlessV1.ErrorInternalServerError("create signing template failed")
	}

	return entity, nil
}

// GetByID retrieves a signing template by ID
func (r *SigningTemplateRepo) GetByID(ctx context.Context, id string) (*ent.SigningTemplate, error) {
	entity, err := r.entClient.Client().SigningTemplate.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get signing template failed: %s", err.Error())
		return nil, paperlessV1.ErrorInternalServerError("get signing template failed")
	}
	return entity, nil
}

// List lists signing templates with optional filters
func (r *SigningTemplateRepo) List(ctx context.Context, tenantID uint32, nameFilter *string, page, pageSize uint32) ([]*ent.SigningTemplate, int, error) {
	query := r.entClient.Client().SigningTemplate.Query().
		Where(signingtemplate.TenantIDEQ(tenantID))

	if nameFilter != nil && *nameFilter != "" {
		query = query.Where(signingtemplate.NameContains(*nameFilter))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count signing templates failed: %s", err.Error())
		return nil, 0, paperlessV1.ErrorInternalServerError("count signing templates failed")
	}

	if page > 0 && pageSize > 0 {
		offset := int((page - 1) * pageSize)
		query = query.Offset(offset).Limit(int(pageSize))
	}

	entities, err := query.Order(ent.Desc(signingtemplate.FieldCreateTime)).All(ctx)
	if err != nil {
		r.log.Errorf("list signing templates failed: %s", err.Error())
		return nil, 0, paperlessV1.ErrorInternalServerError("list signing templates failed")
	}

	return entities, total, nil
}

// Update updates a signing template
func (r *SigningTemplateRepo) Update(ctx context.Context, id string, name, description *string, updatedBy *uint32) (*ent.SigningTemplate, error) {
	builder := r.entClient.Client().SigningTemplate.UpdateOneID(id).
		SetUpdateTime(time.Now())

	if name != nil {
		builder.SetName(*name)
	}
	if description != nil {
		builder.SetDescription(*description)
	}
	if updatedBy != nil {
		builder.SetUpdateBy(*updatedBy)
	}

	entity, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
		}
		if ent.IsConstraintError(err) {
			return nil, paperlessV1.ErrorConflict("signing template with this name already exists")
		}
		r.log.Errorf("update signing template failed: %s", err.Error())
		return nil, paperlessV1.ErrorInternalServerError("update signing template failed")
	}

	return entity, nil
}

// UpdateFields updates the field definitions of a signing template
func (r *SigningTemplateRepo) UpdateFields(ctx context.Context, id string, fields []schema.SigningTemplateFieldDef, updatedBy *uint32) (*ent.SigningTemplate, error) {
	builder := r.entClient.Client().SigningTemplate.UpdateOneID(id).
		SetFields(fields).
		SetUpdateTime(time.Now())

	if updatedBy != nil {
		builder.SetUpdateBy(*updatedBy)
	}

	entity, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
		}
		r.log.Errorf("update signing template fields failed: %s", err.Error())
		return nil, paperlessV1.ErrorInternalServerError("update signing template fields failed")
	}

	return entity, nil
}

// Delete deletes a signing template
func (r *SigningTemplateRepo) Delete(ctx context.Context, id string) error {
	err := r.entClient.Client().SigningTemplate.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
		}
		r.log.Errorf("delete signing template failed: %s", err.Error())
		return paperlessV1.ErrorInternalServerError("delete signing template failed")
	}
	return nil
}

// ToProto converts an ent.SigningTemplate to paperlessV1.SigningTemplate
func (r *SigningTemplateRepo) ToProto(entity *ent.SigningTemplate) *paperlessV1.SigningTemplate {
	if entity == nil {
		return nil
	}

	proto := &paperlessV1.SigningTemplate{
		Id:          entity.ID,
		TenantId:    derefUint32(entity.TenantID),
		Name:        entity.Name,
		Description: entity.Description,
		FileKey:     entity.FileKey,
		FileName:    entity.FileName,
		FileSize:    entity.FileSize,
	}

	// Convert fields
	for _, f := range entity.Fields {
		proto.Fields = append(proto.Fields, &paperlessV1.SigningTemplateField{
			Id:             f.ID,
			Name:           f.Name,
			Type:           fieldTypeToProto(f.Type),
			Required:       f.Required,
			PageNumber:     int32(f.PageNumber),
			XPercent:       f.XPercent,
			YPercent:       f.YPercent,
			WidthPercent:   f.WidthPercent,
			HeightPercent:  f.HeightPercent,
			PrefillStage:   int32(f.PrefillStage),
			RecipientIndex: int32(f.RecipientIndex),
		})
	}

	if entity.CreateBy != nil {
		proto.CreatedBy = entity.CreateBy
	}
	if entity.UpdateBy != nil {
		proto.UpdatedBy = entity.UpdateBy
	}
	if entity.CreateTime != nil && !entity.CreateTime.IsZero() {
		proto.CreateTime = timestamppb.New(*entity.CreateTime)
	}
	if entity.UpdateTime != nil && !entity.UpdateTime.IsZero() {
		proto.UpdateTime = timestamppb.New(*entity.UpdateTime)
	}

	return proto
}

func fieldTypeToProto(t string) paperlessV1.SigningFieldType {
	switch t {
	case "text":
		return paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_TEXT
	case "signature":
		return paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_SIGNATURE
	case "date":
		return paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_DATE
	case "initials":
		return paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_INITIALS
	case "checkbox":
		return paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_CHECKBOX
	case "email":
		return paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_EMAIL
	default:
		return paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_TEXT
	}
}

func protoToFieldType(t paperlessV1.SigningFieldType) string {
	switch t {
	case paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_TEXT:
		return "text"
	case paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_SIGNATURE:
		return "signature"
	case paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_DATE:
		return "date"
	case paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_INITIALS:
		return "initials"
	case paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_CHECKBOX:
		return "checkbox"
	case paperlessV1.SigningFieldType_SIGNING_FIELD_TYPE_EMAIL:
		return "email"
	default:
		return "text"
	}
}

