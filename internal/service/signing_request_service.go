package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/go-tangra/go-tangra-paperless/internal/data"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/schema"
	"github.com/go-tangra/go-tangra-paperless/internal/event"

	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
)

type SigningRequestService struct {
	paperlessV1.UnimplementedPaperlessSigningRequestServiceServer

	log           *log.Helper
	requestRepo   *data.SigningRequestRepo
	templateRepo  *data.SigningTemplateRepo
	recipientRepo *data.SigningRecipientRepo
	storage       *data.StorageClient
	processor     *PDFProcessor
	smtp          *data.SMTPClient
	publisher     *event.Publisher
	appHost       string
}

func NewSigningRequestService(
	ctx *bootstrap.Context,
	requestRepo *data.SigningRequestRepo,
	templateRepo *data.SigningTemplateRepo,
	recipientRepo *data.SigningRecipientRepo,
	storage *data.StorageClient,
	processor *PDFProcessor,
	smtp *data.SMTPClient,
	publisher *event.Publisher,
) *SigningRequestService {
	appHost := os.Getenv("APP_HOST")
	if appHost == "" {
		appHost = "http://localhost:8080"
	}

	return &SigningRequestService{
		log:           ctx.NewLoggerHelper("paperless/service/signing-request"),
		requestRepo:   requestRepo,
		templateRepo:  templateRepo,
		recipientRepo: recipientRepo,
		storage:       storage,
		processor:     processor,
		smtp:          smtp,
		publisher:     publisher,
		appHost:       appHost,
	}
}

// CreateSigningRequest creates a new signing request from a template
func (s *SigningRequestService) CreateSigningRequest(ctx context.Context, req *paperlessV1.CreateSigningRequestRequest) (*paperlessV1.CreateSigningRequestResponse, error) {
	tenantID := getTenantIDFromContext(ctx)
	createdBy := getUserIDAsUint32(ctx)

	// Load template
	template, err := s.templateRepo.GetByID(ctx, req.TemplateId)
	if err != nil {
		return nil, err
	}
	if template == nil {
		return nil, paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
	}

	// Copy template PDF to request path in S3
	requestFileKey := fmt.Sprintf("%d/signing-requests/%s/document.pdf", tenantID, req.Name)
	if err := s.storage.CopyObject(ctx, template.FileKey, requestFileKey); err != nil {
		s.log.Errorf("failed to copy template PDF: %v", err)
		return nil, paperlessV1.ErrorStorageOperationError("failed to copy template PDF")
	}

	// Validate that template fields have positions (set via the visual builder)
	for _, f := range template.Fields {
		if f.PageNumber == 0 || (f.XPercent == 0 && f.YPercent == 0) {
			return nil, paperlessV1.ErrorInvalidTemplatePdf("template fields need positions - edit in builder")
		}
	}

	// Apply stage-1 prefill values directly to the PDF using template positions
	{
		stage1Fills := make([]FieldFill, 0)
		if len(req.FieldValues) > 0 {
			for _, fv := range req.FieldValues {
				for _, tmplField := range template.Fields {
					if (tmplField.ID == fv.FieldId || tmplField.Name == fv.FieldId) && tmplField.PrefillStage == 1 {
						stage1Fills = append(stage1Fills, FieldFill{
							Value:         fv.Value,
							Type:          tmplField.Type,
							PageNumber:    tmplField.PageNumber,
							XPercent:      tmplField.XPercent,
							YPercent:      tmplField.YPercent,
							WidthPercent:  tmplField.WidthPercent,
							HeightPercent: tmplField.HeightPercent,
						})
						break
					}
				}
			}
		}

		if len(stage1Fills) > 0 {
			pdfBytes, err := s.storage.Download(ctx, requestFileKey)
			if err != nil {
				return nil, paperlessV1.ErrorStorageOperationError("failed to download PDF for pre-fill")
			}

			modifiedPDF, err := s.processor.FillFields(pdfBytes, stage1Fills, "", "")
			if err != nil {
				s.log.Warnf("failed to pre-fill stage-1 fields: %v", err)
			} else {
				if _, err := s.storage.UploadRaw(ctx, requestFileKey, modifiedPDF, "application/pdf"); err != nil {
					s.log.Warnf("failed to upload modified PDF: %v", err)
				}
			}
		}
	}

	// Convert ALL field values to schema format (stage 2+ values stored for later)
	// Normalize field references: if FieldId matches a template field's Name, resolve to its ID
	var fieldValues []schema.SigningFieldValueDef
	for _, fv := range req.FieldValues {
		resolvedID := fv.FieldId
		for _, tmplField := range template.Fields {
			if tmplField.Name == fv.FieldId && tmplField.ID != fv.FieldId {
				resolvedID = tmplField.ID
				break
			}
		}
		fieldValues = append(fieldValues, schema.SigningFieldValueDef{
			FieldID:  resolvedID,
			Value:    fv.Value,
			FilledBy: "creator",
		})
	}

	// Parse expiration
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := req.ExpiresAt.AsTime()
		expiresAt = &t
	}

	// Create request record
	entity, err := s.requestRepo.Create(ctx, tenantID, req.TemplateId, req.Name, requestFileKey, req.Message, fieldValues, expiresAt, createdBy)
	if err != nil {
		return nil, err
	}

	// Create recipients and generate tokens
	for _, recipientInput := range req.Recipients {
		token, err := generateSecureToken()
		if err != nil {
			s.log.Errorf("failed to generate token: %v", err)
			return nil, paperlessV1.ErrorInternalServerError("failed to generate signing token")
		}

		// First recipient (order 1) gets PENDING, rest get WAITING
		status := "SIGNING_RECIPIENT_STATUS_WAITING"
		if recipientInput.SigningOrder <= 1 {
			status = "SIGNING_RECIPIENT_STATUS_PENDING"
		}

		order := recipientInput.SigningOrder
		if order <= 0 {
			order = 1
		}

		_, err = s.recipientRepo.Create(ctx, entity.ID, recipientInput.Email, recipientInput.Name, order, status, token)
		if err != nil {
			return nil, err
		}

		// Send email to the first PENDING recipient
		if status == "SIGNING_RECIPIENT_STATUS_PENDING" {
			go s.sendSigningEmail(recipientInput.Email, recipientInput.Name, req.Name, req.Message, token)
		}
	}

	// Re-fetch with recipients
	entity, err = s.requestRepo.GetByID(ctx, entity.ID)
	if err != nil {
		return nil, err
	}

	// Publish signing request created event
	s.publisher.PublishRequestCreated(ctx, &event.SigningRequestCreatedData{
		RequestID:      entity.ID,
		TemplateID:     req.TemplateId,
		TemplateName:   template.Name,
		RecipientCount: len(req.Recipients),
		TenantID:       tenantID,
	})

	return &paperlessV1.CreateSigningRequestResponse{
		Request: s.requestRepo.ToProto(ctx, entity),
	}, nil
}

// GetSigningRequest gets a signing request by ID
func (s *SigningRequestService) GetSigningRequest(ctx context.Context, req *paperlessV1.GetSigningRequestRequest) (*paperlessV1.GetSigningRequestResponse, error) {
	entity, err := s.requestRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, paperlessV1.ErrorSigningRequestNotFound("signing request not found")
	}

	return &paperlessV1.GetSigningRequestResponse{
		Request: s.requestRepo.ToProto(ctx, entity),
	}, nil
}

// ListSigningRequests lists signing requests
func (s *SigningRequestService) ListSigningRequests(ctx context.Context, req *paperlessV1.ListSigningRequestsRequest) (*paperlessV1.ListSigningRequestsResponse, error) {
	tenantID := getTenantIDFromContext(ctx)

	page := uint32(1)
	if req.Page != nil {
		page = *req.Page
	}
	pageSize := uint32(20)
	if req.PageSize != nil {
		pageSize = *req.PageSize
	}

	var status *string
	if req.Status != nil && *req.Status != paperlessV1.SigningRequestStatus_SIGNING_REQUEST_STATUS_UNSPECIFIED {
		s := req.Status.String()
		status = &s
	}

	entities, total, err := s.requestRepo.List(ctx, tenantID, status, req.TemplateId, req.NameFilter, page, pageSize)
	if err != nil {
		return nil, err
	}

	requests := make([]*paperlessV1.SigningRequest, 0, len(entities))
	for _, entity := range entities {
		requests = append(requests, s.requestRepo.ToProto(ctx, entity))
	}

	return &paperlessV1.ListSigningRequestsResponse{
		Requests: requests,
		Total:    uint32(total),
	}, nil
}

// CancelSigningRequest cancels a signing request
func (s *SigningRequestService) CancelSigningRequest(ctx context.Context, req *paperlessV1.CancelSigningRequestRequest) (*paperlessV1.CancelSigningRequestResponse, error) {
	entity, err := s.requestRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, paperlessV1.ErrorSigningRequestNotFound("signing request not found")
	}

	// Can only cancel PENDING requests
	if string(entity.Status) != "SIGNING_REQUEST_STATUS_PENDING" {
		return nil, paperlessV1.ErrorSigningAlreadyCompleted("signing request is not in pending state")
	}

	// Cancel all pending/waiting recipients
	if err := s.recipientRepo.CancelPendingByRequestID(ctx, req.Id); err != nil {
		return nil, err
	}

	// Update request status
	if err := s.requestRepo.UpdateStatus(ctx, req.Id, "SIGNING_REQUEST_STATUS_CANCELLED"); err != nil {
		return nil, err
	}

	// Publish signing request cancelled event
	tenantID := getTenantIDFromContext(ctx)
	s.publisher.PublishRequestCancelled(ctx, &event.SigningRequestCancelledData{
		RequestID: req.Id,
		TenantID:  tenantID,
	})

	// Re-fetch updated entity
	entity, err = s.requestRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &paperlessV1.CancelSigningRequestResponse{
		Request: s.requestRepo.ToProto(ctx, entity),
	}, nil
}

// ResendSigningEmail resends the signing email to a specific recipient
func (s *SigningRequestService) ResendSigningEmail(ctx context.Context, req *paperlessV1.ResendSigningEmailRequest) (*emptypb.Empty, error) {
	entity, err := s.requestRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, paperlessV1.ErrorSigningRequestNotFound("signing request not found")
	}

	// Find the recipient
	recipients, err := s.recipientRepo.ListByRequestID(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	var targetRecipient *struct {
		Email, Name, Token string
	}
	for _, rec := range recipients {
		if rec.ID == req.RecipientId {
			if string(rec.Status) != "SIGNING_RECIPIENT_STATUS_PENDING" {
				return nil, paperlessV1.ErrorSigningAlreadyCompleted("recipient is not in pending state")
			}
			targetRecipient = &struct {
				Email, Name, Token string
			}{rec.Email, rec.Name, rec.Token}
			break
		}
	}
	if targetRecipient == nil {
		return nil, paperlessV1.ErrorSigningSessionNotFound("recipient not found")
	}

	go s.sendSigningEmail(targetRecipient.Email, targetRecipient.Name, entity.Name, entity.Message, targetRecipient.Token)

	return &emptypb.Empty{}, nil
}

// DownloadSignedDocument returns a proxy URL for the signed document
func (s *SigningRequestService) DownloadSignedDocument(ctx context.Context, req *paperlessV1.DownloadSignedDocumentRequest) (*paperlessV1.DownloadSignedDocumentResponse, error) {
	entity, err := s.requestRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, paperlessV1.ErrorSigningRequestNotFound("signing request not found")
	}

	// Generate a short-lived download token
	expiresIn := time.Hour
	expiresAt := time.Now().Add(expiresIn)
	token := GenerateDownloadToken(req.Id, expiresAt)

	proxyURL := fmt.Sprintf("/public/v1/signing/requests/%s/download?token=%s&expires=%d", req.Id, token, expiresAt.Unix())

	return &paperlessV1.DownloadSignedDocumentResponse{
		Url:       proxyURL,
		ExpiresAt: timestamppb.New(expiresAt),
	}, nil
}

// GetDocumentBytes downloads the signed document bytes for proxying
func (s *SigningRequestService) GetDocumentBytes(ctx context.Context, requestID string) ([]byte, string, error) {
	entity, err := s.requestRepo.GetByID(ctx, requestID)
	if err != nil {
		return nil, "", err
	}
	if entity == nil {
		return nil, "", paperlessV1.ErrorSigningRequestNotFound("signing request not found")
	}

	fileKey := entity.SignedFileKey
	if fileKey == "" {
		fileKey = entity.OriginalFileKey
	}

	pdfBytes, err := s.storage.Download(ctx, fileKey)
	if err != nil {
		s.log.Errorf("failed to download signed document: %v", err)
		return nil, "", paperlessV1.ErrorStorageOperationError("failed to download document")
	}

	return pdfBytes, entity.Name + ".pdf", nil
}

// sendSigningEmail sends a signing invitation email
func (s *SigningRequestService) sendSigningEmail(email, name, requestName, message, token string) {
	signingLink := fmt.Sprintf("%s/#/signing/%s", s.appHost, token)

	subject := fmt.Sprintf("Document signing request: %s", requestName)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
  <div style="background: #f8f9fa; border-radius: 8px; padding: 30px;">
    <h2 style="color: #333; margin-top: 0;">Document Signing Request</h2>
    <p>Hello %s,</p>
    <p>You have been invited to sign the document: <strong>%s</strong></p>
    %s
    <div style="text-align: center; margin: 30px 0;">
      <a href="%s" style="background: #1890ff; color: white; padding: 12px 30px; border-radius: 6px; text-decoration: none; font-weight: bold;">
        Review and Sign
      </a>
    </div>
    <p style="color: #666; font-size: 13px;">
      If the button doesn't work, copy this link into your browser:<br>
      <a href="%s">%s</a>
    </p>
    <hr style="border: none; border-top: 1px solid #e8e8e8; margin: 20px 0;">
    <p style="color: #999; font-size: 12px;">
      This is an automated message. Please do not reply to this email.
    </p>
  </div>
</body>
</html>`, name, requestName, formatMessage(message), signingLink, signingLink, signingLink)

	if err := s.smtp.Send(email, subject, htmlBody); err != nil {
		s.log.Errorf("failed to send signing email to %s: %v", email, err)
	} else {
		s.log.Infof("signing email sent to %s for request %s", email, requestName)
	}
}

func formatMessage(message string) string {
	if message == "" {
		return ""
	}
	return fmt.Sprintf(`<div style="background: #fff; border-left: 3px solid #1890ff; padding: 10px 15px; margin: 15px 0;">
      <p style="margin: 0; color: #555;">%s</p>
    </div>`, message)
}

// generateSecureToken generates a 64-character hex token
func generateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
