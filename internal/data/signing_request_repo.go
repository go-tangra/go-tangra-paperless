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
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/signingrequest"

	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
)

type SigningRequestRepo struct {
	entClient     *entCrud.EntClient[*ent.Client]
	recipientRepo *SigningRecipientRepo
	templateRepo  *SigningTemplateRepo
	log           *log.Helper
}

func NewSigningRequestRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client], recipientRepo *SigningRecipientRepo, templateRepo *SigningTemplateRepo) *SigningRequestRepo {
	return &SigningRequestRepo{
		log:           ctx.NewLoggerHelper("paperless/signing-request/repo"),
		entClient:     entClient,
		recipientRepo: recipientRepo,
		templateRepo:  templateRepo,
	}
}

// Create creates a new signing request
func (r *SigningRequestRepo) Create(ctx context.Context, tenantID uint32, templateID, name, originalFileKey, message, signingType string, fieldValues []schema.SigningFieldValueDef, expiresAt *time.Time, createdBy *uint32) (*ent.SigningRequest, error) {
	id := uuid.New().String()

	builder := r.entClient.Client().SigningRequest.Create().
		SetID(id).
		SetTenantID(tenantID).
		SetTemplateID(templateID).
		SetName(name).
		SetStatus(signingrequest.StatusSIGNING_REQUEST_STATUS_PENDING).
		SetOriginalFileKey(originalFileKey).
		SetCreateTime(time.Now())

	if signingType != "" {
		builder.SetSigningType(signingrequest.SigningType(signingType))
	}

	if message != "" {
		builder.SetMessage(message)
	}
	if fieldValues != nil {
		builder.SetFieldValues(fieldValues)
	}
	if expiresAt != nil {
		builder.SetExpiresAt(*expiresAt)
	}
	if createdBy != nil {
		builder.SetCreateBy(*createdBy)
	}

	entity, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf("create signing request failed: %s", err.Error())
		return nil, paperlessV1.ErrorInternalServerError("create signing request failed")
	}

	return entity, nil
}

// GetByID retrieves a signing request by ID with recipients
func (r *SigningRequestRepo) GetByID(ctx context.Context, id string) (*ent.SigningRequest, error) {
	entity, err := r.entClient.Client().SigningRequest.
		Query().
		Where(signingrequest.IDEQ(id)).
		WithRecipients().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get signing request failed: %s", err.Error())
		return nil, paperlessV1.ErrorInternalServerError("get signing request failed")
	}
	return entity, nil
}

// List lists signing requests with optional filters
func (r *SigningRequestRepo) List(ctx context.Context, tenantID uint32, status *string, templateID, nameFilter *string, page, pageSize uint32) ([]*ent.SigningRequest, int, error) {
	query := r.entClient.Client().SigningRequest.Query().
		Where(signingrequest.TenantIDEQ(tenantID))

	if status != nil && *status != "" {
		query = query.Where(signingrequest.StatusEQ(signingrequest.Status(*status)))
	}
	if templateID != nil && *templateID != "" {
		query = query.Where(signingrequest.TemplateIDEQ(*templateID))
	}
	if nameFilter != nil && *nameFilter != "" {
		query = query.Where(signingrequest.NameContains(*nameFilter))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count signing requests failed: %s", err.Error())
		return nil, 0, paperlessV1.ErrorInternalServerError("count signing requests failed")
	}

	if page > 0 && pageSize > 0 {
		offset := int((page - 1) * pageSize)
		query = query.Offset(offset).Limit(int(pageSize))
	}

	entities, err := query.
		WithRecipients().
		Order(ent.Desc(signingrequest.FieldCreateTime)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list signing requests failed: %s", err.Error())
		return nil, 0, paperlessV1.ErrorInternalServerError("list signing requests failed")
	}

	return entities, total, nil
}

// UpdateStatus updates the status of a signing request
func (r *SigningRequestRepo) UpdateStatus(ctx context.Context, id string, status string) error {
	_, err := r.entClient.Client().SigningRequest.UpdateOneID(id).
		SetStatus(signingrequest.Status(status)).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return paperlessV1.ErrorSigningRequestNotFound("signing request not found")
		}
		r.log.Errorf("update signing request status failed: %s", err.Error())
		return paperlessV1.ErrorInternalServerError("update signing request status failed")
	}
	return nil
}

// SetSignedFileKey sets the signed file key on a signing request
func (r *SigningRequestRepo) SetSignedFileKey(ctx context.Context, id, signedFileKey string) error {
	_, err := r.entClient.Client().SigningRequest.UpdateOneID(id).
		SetSignedFileKey(signedFileKey).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return paperlessV1.ErrorSigningRequestNotFound("signing request not found")
		}
		r.log.Errorf("set signed file key failed: %s", err.Error())
		return paperlessV1.ErrorInternalServerError("set signed file key failed")
	}
	return nil
}

// UpdateFieldValues updates the field values of a signing request
func (r *SigningRequestRepo) UpdateFieldValues(ctx context.Context, id string, fieldValues []schema.SigningFieldValueDef) error {
	_, err := r.entClient.Client().SigningRequest.UpdateOneID(id).
		SetFieldValues(fieldValues).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return paperlessV1.ErrorSigningRequestNotFound("signing request not found")
		}
		r.log.Errorf("update field values failed: %s", err.Error())
		return paperlessV1.ErrorInternalServerError("update field values failed")
	}
	return nil
}

// Delete deletes a signing request and its recipients
func (r *SigningRequestRepo) Delete(ctx context.Context, id string) error {
	// Delete recipients first
	if err := r.recipientRepo.DeleteByRequestID(ctx, id); err != nil {
		return err
	}

	err := r.entClient.Client().SigningRequest.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return paperlessV1.ErrorSigningRequestNotFound("signing request not found")
		}
		r.log.Errorf("delete signing request failed: %s", err.Error())
		return paperlessV1.ErrorInternalServerError("delete signing request failed")
	}
	return nil
}

// ToProto converts an ent.SigningRequest to paperlessV1.SigningRequest
func (r *SigningRequestRepo) ToProto(ctx context.Context, entity *ent.SigningRequest) *paperlessV1.SigningRequest {
	if entity == nil {
		return nil
	}

	proto := &paperlessV1.SigningRequest{
		Id:              entity.ID,
		TenantId:        derefUint32(entity.TenantID),
		TemplateId:      entity.TemplateID,
		Name:            entity.Name,
		Status:          signingRequestStatusToProto(string(entity.Status)),
		OriginalFileKey: entity.OriginalFileKey,
		SignedFileKey:    entity.SignedFileKey,
		Message:         entity.Message,
		SigningType:      signingRequestTypeToProto(string(entity.SigningType)),
	}

	// Get template name
	tmpl, err := r.templateRepo.GetByID(ctx, entity.TemplateID)
	if err == nil && tmpl != nil {
		proto.TemplateName = tmpl.Name
	}

	// Convert field values
	for _, fv := range entity.FieldValues {
		proto.FieldValues = append(proto.FieldValues, &paperlessV1.SigningFieldValue{
			FieldId:   fv.FieldID,
			FieldName: fv.FieldName,
			Value:     fv.Value,
			FilledBy:  fv.FilledBy,
		})
	}

	// Convert recipients from eager-loaded edges
	if entity.Edges.Recipients != nil {
		for _, rec := range entity.Edges.Recipients {
			proto.Recipients = append(proto.Recipients, r.recipientRepo.ToProto(rec))
		}
	}

	if entity.ExpiresAt != nil {
		proto.ExpiresAt = timestamppb.New(*entity.ExpiresAt)
	}
	if entity.CreateBy != nil {
		proto.CreatedBy = entity.CreateBy
	}
	if entity.CreateTime != nil && !entity.CreateTime.IsZero() {
		proto.CreateTime = timestamppb.New(*entity.CreateTime)
	}
	if entity.UpdateTime != nil && !entity.UpdateTime.IsZero() {
		proto.UpdateTime = timestamppb.New(*entity.UpdateTime)
	}

	return proto
}

func signingRequestTypeToProto(signingType string) paperlessV1.SigningRequestType {
	switch signingType {
	case "SIGNING_REQUEST_TYPE_EXTERNAL":
		return paperlessV1.SigningRequestType_SIGNING_REQUEST_TYPE_EXTERNAL
	case "SIGNING_REQUEST_TYPE_INTERNAL":
		return paperlessV1.SigningRequestType_SIGNING_REQUEST_TYPE_INTERNAL
	default:
		return paperlessV1.SigningRequestType_SIGNING_REQUEST_TYPE_UNSPECIFIED
	}
}

func signingRequestStatusToProto(status string) paperlessV1.SigningRequestStatus {
	switch status {
	case "SIGNING_REQUEST_STATUS_DRAFT":
		return paperlessV1.SigningRequestStatus_SIGNING_REQUEST_STATUS_DRAFT
	case "SIGNING_REQUEST_STATUS_PENDING":
		return paperlessV1.SigningRequestStatus_SIGNING_REQUEST_STATUS_PENDING
	case "SIGNING_REQUEST_STATUS_COMPLETED":
		return paperlessV1.SigningRequestStatus_SIGNING_REQUEST_STATUS_COMPLETED
	case "SIGNING_REQUEST_STATUS_CANCELLED":
		return paperlessV1.SigningRequestStatus_SIGNING_REQUEST_STATUS_CANCELLED
	case "SIGNING_REQUEST_STATUS_EXPIRED":
		return paperlessV1.SigningRequestStatus_SIGNING_REQUEST_STATUS_EXPIRED
	case "SIGNING_REQUEST_STATUS_REVOKED":
		return paperlessV1.SigningRequestStatus_SIGNING_REQUEST_STATUS_REVOKED
	default:
		return paperlessV1.SigningRequestStatus_SIGNING_REQUEST_STATUS_UNSPECIFIED
	}
}
