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
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/signingrecipient"

	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
)

type SigningRecipientRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *log.Helper
}

func NewSigningRecipientRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *SigningRecipientRepo {
	return &SigningRecipientRepo{
		log:       ctx.NewLoggerHelper("paperless/signing-recipient/repo"),
		entClient: entClient,
	}
}

// Create creates a new signing recipient
func (r *SigningRecipientRepo) Create(ctx context.Context, signingRequestID, email, name string, signingOrder int32, status string, token string) (*ent.SigningRecipient, error) {
	id := uuid.New().String()

	entity, err := r.entClient.Client().SigningRecipient.Create().
		SetID(id).
		SetSigningRequestID(signingRequestID).
		SetEmail(email).
		SetName(name).
		SetSigningOrder(signingOrder).
		SetStatus(signingrecipient.Status(status)).
		SetToken(token).
		SetCreateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("create signing recipient failed: %s", err.Error())
		return nil, paperlessV1.ErrorInternalServerError("create signing recipient failed")
	}

	return entity, nil
}

// GetByToken retrieves a signing recipient by their unique token
func (r *SigningRecipientRepo) GetByToken(ctx context.Context, token string) (*ent.SigningRecipient, error) {
	entity, err := r.entClient.Client().SigningRecipient.Query().
		Where(signingrecipient.TokenEQ(token)).
		WithSigningRequest().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get signing recipient by token failed: %s", err.Error())
		return nil, paperlessV1.ErrorInternalServerError("get signing recipient failed")
	}
	return entity, nil
}

// ListByRequestID lists all recipients for a signing request
func (r *SigningRecipientRepo) ListByRequestID(ctx context.Context, signingRequestID string) ([]*ent.SigningRecipient, error) {
	entities, err := r.entClient.Client().SigningRecipient.Query().
		Where(signingrecipient.SigningRequestIDEQ(signingRequestID)).
		Order(ent.Asc(signingrecipient.FieldSigningOrder)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list signing recipients failed: %s", err.Error())
		return nil, paperlessV1.ErrorInternalServerError("list signing recipients failed")
	}
	return entities, nil
}

// UpdateStatus updates a recipient's status
func (r *SigningRecipientRepo) UpdateStatus(ctx context.Context, id string, status string) error {
	_, err := r.entClient.Client().SigningRecipient.UpdateOneID(id).
		SetStatus(signingrecipient.Status(status)).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return paperlessV1.ErrorSigningSessionNotFound("signing recipient not found")
		}
		r.log.Errorf("update signing recipient status failed: %s", err.Error())
		return paperlessV1.ErrorInternalServerError("update signing recipient status failed")
	}
	return nil
}

// MarkSigned marks a recipient as having completed signing
func (r *SigningRecipientRepo) MarkSigned(ctx context.Context, id string, signedIP string, signatureData []byte) error {
	now := time.Now()
	builder := r.entClient.Client().SigningRecipient.UpdateOneID(id).
		SetStatus(signingrecipient.StatusSIGNING_RECIPIENT_STATUS_COMPLETED).
		SetSignedAt(now).
		SetUpdateTime(now)

	if signedIP != "" {
		builder.SetSignedIP(signedIP)
	}
	if signatureData != nil {
		builder.SetSignatureData(signatureData)
	}

	_, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return paperlessV1.ErrorSigningSessionNotFound("signing recipient not found")
		}
		r.log.Errorf("mark signed failed: %s", err.Error())
		return paperlessV1.ErrorInternalServerError("mark signed failed")
	}
	return nil
}

// GetNextPending gets the next recipient in signing order with WAITING status
func (r *SigningRecipientRepo) GetNextPending(ctx context.Context, signingRequestID string) (*ent.SigningRecipient, error) {
	entity, err := r.entClient.Client().SigningRecipient.Query().
		Where(
			signingrecipient.SigningRequestIDEQ(signingRequestID),
			signingrecipient.StatusEQ(signingrecipient.StatusSIGNING_RECIPIENT_STATUS_WAITING),
		).
		Order(ent.Asc(signingrecipient.FieldSigningOrder)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get next pending recipient failed: %s", err.Error())
		return nil, paperlessV1.ErrorInternalServerError("get next pending recipient failed")
	}
	return entity, nil
}

// DeleteByRequestID deletes all recipients for a signing request
func (r *SigningRecipientRepo) DeleteByRequestID(ctx context.Context, signingRequestID string) error {
	_, err := r.entClient.Client().SigningRecipient.Delete().
		Where(signingrecipient.SigningRequestIDEQ(signingRequestID)).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete signing recipients failed: %s", err.Error())
		return paperlessV1.ErrorInternalServerError("delete signing recipients failed")
	}
	return nil
}

// CancelPendingByRequestID cancels all WAITING and PENDING recipients for a request
func (r *SigningRecipientRepo) CancelPendingByRequestID(ctx context.Context, signingRequestID string) error {
	_, err := r.entClient.Client().SigningRecipient.Update().
		Where(
			signingrecipient.SigningRequestIDEQ(signingRequestID),
			signingrecipient.StatusIn(
				signingrecipient.StatusSIGNING_RECIPIENT_STATUS_WAITING,
				signingrecipient.StatusSIGNING_RECIPIENT_STATUS_PENDING,
			),
		).
		SetStatus(signingrecipient.StatusSIGNING_RECIPIENT_STATUS_DECLINED).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("cancel pending recipients failed: %s", err.Error())
		return paperlessV1.ErrorInternalServerError("cancel pending recipients failed")
	}
	return nil
}

// ToProto converts an ent.SigningRecipient to paperlessV1.SigningRecipient
func (r *SigningRecipientRepo) ToProto(entity *ent.SigningRecipient) *paperlessV1.SigningRecipient {
	if entity == nil {
		return nil
	}

	proto := &paperlessV1.SigningRecipient{
		Id:           entity.ID,
		Email:        entity.Email,
		Name:         entity.Name,
		SigningOrder: entity.SigningOrder,
		Status:       signingRecipientStatusToProto(string(entity.Status)),
	}

	if entity.SignedAt != nil {
		proto.SignedAt = timestamppb.New(*entity.SignedAt)
	}

	return proto
}

func signingRecipientStatusToProto(status string) paperlessV1.SigningRecipientStatus {
	switch status {
	case "SIGNING_RECIPIENT_STATUS_WAITING":
		return paperlessV1.SigningRecipientStatus_SIGNING_RECIPIENT_STATUS_WAITING
	case "SIGNING_RECIPIENT_STATUS_PENDING":
		return paperlessV1.SigningRecipientStatus_SIGNING_RECIPIENT_STATUS_PENDING
	case "SIGNING_RECIPIENT_STATUS_COMPLETED":
		return paperlessV1.SigningRecipientStatus_SIGNING_RECIPIENT_STATUS_COMPLETED
	case "SIGNING_RECIPIENT_STATUS_DECLINED":
		return paperlessV1.SigningRecipientStatus_SIGNING_RECIPIENT_STATUS_DECLINED
	default:
		return paperlessV1.SigningRecipientStatus_SIGNING_RECIPIENT_STATUS_UNSPECIFIED
	}
}
