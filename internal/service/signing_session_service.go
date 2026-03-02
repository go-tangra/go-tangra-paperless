package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/go-tangra/go-tangra-paperless/internal/data"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/schema"
	"github.com/go-tangra/go-tangra-paperless/internal/event"

	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
)

type SigningSessionService struct {
	paperlessV1.UnimplementedPaperlessSigningSessionServiceServer

	log           *log.Helper
	recipientRepo *data.SigningRecipientRepo
	requestRepo   *data.SigningRequestRepo
	templateRepo  *data.SigningTemplateRepo
	storage       *data.StorageClient
	processor     *PDFProcessor
	smtp          *data.SMTPClient
	publisher     *event.Publisher
}

func NewSigningSessionService(
	ctx *bootstrap.Context,
	recipientRepo *data.SigningRecipientRepo,
	requestRepo *data.SigningRequestRepo,
	templateRepo *data.SigningTemplateRepo,
	storage *data.StorageClient,
	processor *PDFProcessor,
	smtp *data.SMTPClient,
	publisher *event.Publisher,
) *SigningSessionService {
	return &SigningSessionService{
		log:           ctx.NewLoggerHelper("paperless/service/signing-session"),
		recipientRepo: recipientRepo,
		requestRepo:   requestRepo,
		templateRepo:  templateRepo,
		storage:       storage,
		processor:     processor,
		smtp:          smtp,
		publisher:     publisher,
	}
}

// GetSigningSession retrieves signing session info for a recipient by token
func (s *SigningSessionService) GetSigningSession(ctx context.Context, req *paperlessV1.GetSigningSessionRequest) (*paperlessV1.GetSigningSessionResponse, error) {
	// Look up recipient by token
	recipient, err := s.recipientRepo.GetByToken(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	if recipient == nil {
		return nil, paperlessV1.ErrorSigningSessionNotFound("signing session not found")
	}

	// Load the signing request
	signingRequest := recipient.Edges.SigningRequest
	if signingRequest == nil {
		signingRequest, err = s.requestRepo.GetByID(ctx, recipient.SigningRequestID)
		if err != nil || signingRequest == nil {
			return nil, paperlessV1.ErrorSigningSessionNotFound("signing request not found")
		}
	}

	// Check status
	if string(recipient.Status) != "SIGNING_RECIPIENT_STATUS_PENDING" {
		if string(recipient.Status) == "SIGNING_RECIPIENT_STATUS_COMPLETED" {
			return nil, paperlessV1.ErrorSigningAlreadyCompleted("you have already signed this document")
		}
		return nil, paperlessV1.ErrorSigningSessionNotFound("signing session is not active")
	}

	// Check expiration
	if signingRequest.ExpiresAt != nil && signingRequest.ExpiresAt.Before(time.Now()) {
		return nil, paperlessV1.ErrorSigningSessionExpired("this signing request has expired")
	}

	// Check request status
	if string(signingRequest.Status) == "SIGNING_REQUEST_STATUS_CANCELLED" {
		return nil, paperlessV1.ErrorSigningSessionNotFound("this signing request has been cancelled")
	}

	// Load template for field definitions
	template, err := s.templateRepo.GetByID(ctx, signingRequest.TemplateID)
	if err != nil || template == nil {
		return nil, paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
	}

	// Build proxy URL for the document (served through HTTP server, not direct S3)
	documentURL := fmt.Sprintf("/public/v1/signing/sessions/%s/document", req.Token)

	// Build fields list filtered by recipient, with pre-filled values
	signingOrder := int(recipient.SigningOrder)
	fields := make([]*paperlessV1.SigningSessionField, 0, len(template.Fields))
	for _, tmplField := range template.Fields {
		// Skip prefill fields — admin-only, already applied to PDF
		if tmplField.PrefillStage > 0 {
			continue
		}
		// Skip fields assigned to other recipients
		if tmplField.RecipientIndex > 0 && tmplField.RecipientIndex != signingOrder {
			continue
		}
		// Include: RecipientIndex==0 (all) or RecipientIndex==signingOrder

		field := &paperlessV1.SigningSessionField{
			FieldId:        tmplField.ID,
			Name:           tmplField.Name,
			Type:           fieldTypeToSessionProto(tmplField.Type),
			Required:       tmplField.Required,
			RecipientIndex: int32(tmplField.RecipientIndex),
			PageNumber:     int32(tmplField.PageNumber),
			XPercent:       tmplField.XPercent,
			YPercent:       tmplField.YPercent,
			WidthPercent:   tmplField.WidthPercent,
			HeightPercent:  tmplField.HeightPercent,
		}

		// Check for pre-filled values (match by ID or Name for backwards compatibility)
		for _, fv := range signingRequest.FieldValues {
			if fv.FieldID == tmplField.ID || fv.FieldID == tmplField.Name {
				field.PrefilledValue = fv.Value
				break
			}
		}

		fields = append(fields, field)
	}

	resp := &paperlessV1.GetSigningSessionResponse{
		RequestName:    signingRequest.Name,
		DocumentUrl:    documentURL,
		RecipientName:  recipient.Name,
		RecipientEmail: recipient.Email,
		Fields:         fields,
		Message:        signingRequest.Message,
	}

	if signingRequest.ExpiresAt != nil {
		resp.ExpiresAt = timestamppb.New(*signingRequest.ExpiresAt)
	}

	return resp, nil
}

// GetDocument retrieves the PDF bytes for a signing session (proxied through HTTP server)
func (s *SigningSessionService) GetDocument(ctx context.Context, token string) ([]byte, string, error) {
	recipient, err := s.recipientRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, "", err
	}
	if recipient == nil {
		return nil, "", paperlessV1.ErrorSigningSessionNotFound("signing session not found")
	}

	// Verify recipient is pending
	if string(recipient.Status) != "SIGNING_RECIPIENT_STATUS_PENDING" {
		if string(recipient.Status) == "SIGNING_RECIPIENT_STATUS_COMPLETED" {
			return nil, "", paperlessV1.ErrorSigningAlreadyCompleted("signing already completed")
		}
		return nil, "", paperlessV1.ErrorSigningSessionNotFound("signing session is not active")
	}

	signingRequest := recipient.Edges.SigningRequest
	if signingRequest == nil {
		signingRequest, err = s.requestRepo.GetByID(ctx, recipient.SigningRequestID)
		if err != nil || signingRequest == nil {
			return nil, "", paperlessV1.ErrorSigningSessionNotFound("signing request not found")
		}
	}

	// Check expiration
	if signingRequest.ExpiresAt != nil && signingRequest.ExpiresAt.Before(time.Now()) {
		return nil, "", paperlessV1.ErrorSigningSessionExpired("this signing request has expired")
	}

	pdfBytes, err := s.storage.Download(ctx, signingRequest.OriginalFileKey)
	if err != nil {
		s.log.Errorf("failed to download document: %v", err)
		return nil, "", paperlessV1.ErrorStorageOperationError("failed to download document")
	}

	return pdfBytes, signingRequest.Name + ".pdf", nil
}

// SubmitSigning processes a recipient's signing submission
func (s *SigningSessionService) SubmitSigning(ctx context.Context, req *paperlessV1.SubmitSigningRequest) (*paperlessV1.SubmitSigningResponse, error) {
	// Look up recipient by token
	recipient, err := s.recipientRepo.GetByToken(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	if recipient == nil {
		return nil, paperlessV1.ErrorSigningSessionNotFound("signing session not found")
	}

	// Verify status
	if string(recipient.Status) != "SIGNING_RECIPIENT_STATUS_PENDING" {
		return nil, paperlessV1.ErrorSigningAlreadyCompleted("signing has already been completed or is not active")
	}

	// Load signing request
	signingRequest, err := s.requestRepo.GetByID(ctx, recipient.SigningRequestID)
	if err != nil || signingRequest == nil {
		return nil, paperlessV1.ErrorSigningRequestNotFound("signing request not found")
	}

	// Check expiration
	if signingRequest.ExpiresAt != nil && signingRequest.ExpiresAt.Before(time.Now()) {
		return nil, paperlessV1.ErrorSigningSessionExpired("this signing request has expired")
	}

	// Load template
	template, err := s.templateRepo.GetByID(ctx, signingRequest.TemplateID)
	if err != nil || template == nil {
		return nil, paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
	}

	// Download current PDF
	pdfBytes, err := s.storage.Download(ctx, signingRequest.OriginalFileKey)
	if err != nil {
		s.log.Errorf("failed to download PDF: %v", err)
		return nil, paperlessV1.ErrorStorageOperationError("failed to download document")
	}

	// Build fill instructions using template positions as source of truth
	fills := make([]FieldFill, 0, len(req.FieldValues)+len(template.Fields))
	matchedFieldIDs := make(map[string]bool)

	// Match submitted field values to template fields, using template positions
	for _, fv := range req.FieldValues {
		for _, tmplField := range template.Fields {
			if tmplField.ID == fv.FieldId {
				fill := FieldFill{
					Value:        fv.Value,
					Type:         tmplField.Type,
					PageNumber:   tmplField.PageNumber,
					XPercent:     tmplField.XPercent,
					YPercent:     tmplField.YPercent,
					WidthPercent: tmplField.WidthPercent,
					HeightPercent: tmplField.HeightPercent,
				}
				if (tmplField.Type == "signature" || tmplField.Type == "initials") && len(req.SignatureImage) > 0 {
					fill.ImageData = req.SignatureImage
				}
				fills = append(fills, fill)
				matchedFieldIDs[tmplField.ID] = true
				break
			}
		}
	}

	// Add signature/initials fields that weren't submitted as field values
	// (frontend sends signatureImage separately, not as a field value)
	// Only stamp fields assigned to the current recipient (or shared fields with recipientIndex=0)
	for _, tmplField := range template.Fields {
		if matchedFieldIDs[tmplField.ID] {
			continue
		}
		if tmplField.RecipientIndex != 0 && tmplField.RecipientIndex != int(recipient.SigningOrder) {
			continue
		}
		if (tmplField.Type == "signature" || tmplField.Type == "initials") && len(req.SignatureImage) > 0 {
			fills = append(fills, FieldFill{
				Value:         "[Signed]",
				Type:          tmplField.Type,
				ImageData:     req.SignatureImage,
				PageNumber:    tmplField.PageNumber,
				XPercent:      tmplField.XPercent,
				YPercent:      tmplField.YPercent,
				WidthPercent:  tmplField.WidthPercent,
				HeightPercent: tmplField.HeightPercent,
			})
		}
	}

	// Fill fields in PDF
	if len(fills) > 0 {
		modifiedPDF, err := s.processor.FillFields(pdfBytes, fills, recipient.Name, recipient.Email)
		if err != nil {
			s.log.Errorf("failed to fill fields: %v", err)
			return nil, paperlessV1.ErrorInternalServerError("failed to process document")
		}

		// Upload modified PDF back
		if _, err := s.storage.UploadRaw(ctx, signingRequest.OriginalFileKey, modifiedPDF, "application/pdf"); err != nil {
			s.log.Errorf("failed to upload modified PDF: %v", err)
			return nil, paperlessV1.ErrorStorageOperationError("failed to save signed document")
		}
	}

	// Extract client IP from context
	clientIP := extractClientIP(ctx)

	// Mark recipient as completed
	if err := s.recipientRepo.MarkSigned(ctx, recipient.ID, clientIP, req.SignatureImage); err != nil {
		return nil, err
	}

	// Update field values on the request
	existingValues := signingRequest.FieldValues
	for _, fv := range req.FieldValues {
		existingValues = append(existingValues, schema.SigningFieldValueDef{
			FieldID:  fv.FieldId,
			Value:    fv.Value,
			FilledBy: recipient.Email,
		})
	}
	if err := s.requestRepo.UpdateFieldValues(ctx, signingRequest.ID, existingValues); err != nil {
		s.log.Warnf("failed to update field values: %v", err)
	}

	// Advance to next recipient
	nextRecipient, err := s.recipientRepo.GetNextPending(ctx, signingRequest.ID)
	if err != nil {
		s.log.Errorf("failed to get next recipient: %v", err)
	}

	// Count remaining recipients for event data
	allRecipients, _ := s.recipientRepo.ListByRequestID(ctx, signingRequest.ID)
	remainingCount := 0
	for _, r := range allRecipients {
		if string(r.Status) == "SIGNING_RECIPIENT_STATUS_WAITING" || string(r.Status) == "SIGNING_RECIPIENT_STATUS_PENDING" {
			remainingCount++
		}
	}

	var tenantID uint32
	if signingRequest.TenantID != nil {
		tenantID = *signingRequest.TenantID
	}

	if nextRecipient != nil {
		// Apply staged prefill values for the next recipient's stage
		s.applyStagedPrefills(ctx, signingRequest, template, int(nextRecipient.SigningOrder))

		// Set next recipient to PENDING and send email
		if err := s.recipientRepo.UpdateStatus(ctx, nextRecipient.ID, "SIGNING_RECIPIENT_STATUS_PENDING"); err != nil {
			s.log.Errorf("failed to update next recipient status: %v", err)
		}
		go s.sendNextSigningEmail(nextRecipient.Email, nextRecipient.Name, signingRequest.Name, signingRequest.Message, nextRecipient.Token)

		// Publish recipient signed event
		s.publisher.PublishRecipientSigned(ctx, &event.SigningRecipientSignedData{
			RequestID:      signingRequest.ID,
			RecipientEmail: recipient.Email,
			RecipientName:  recipient.Name,
			SigningOrder:   recipient.SigningOrder,
			RemainingCount: remainingCount,
			TenantID:       tenantID,
		})

		return &paperlessV1.SubmitSigningResponse{
			Completed: false,
			Message:   "Thank you for signing. The document has been forwarded to the next signer.",
		}, nil
	}

	// No more recipients - mark request as COMPLETED
	if err := s.requestRepo.UpdateStatus(ctx, signingRequest.ID, "SIGNING_REQUEST_STATUS_COMPLETED"); err != nil {
		s.log.Errorf("failed to mark request as completed: %v", err)
	}

	// Set signed file key
	if err := s.requestRepo.SetSignedFileKey(ctx, signingRequest.ID, signingRequest.OriginalFileKey); err != nil {
		s.log.Warnf("failed to set signed file key: %v", err)
	}

	// Publish recipient signed event (last signer)
	s.publisher.PublishRecipientSigned(ctx, &event.SigningRecipientSignedData{
		RequestID:      signingRequest.ID,
		RecipientEmail: recipient.Email,
		RecipientName:  recipient.Name,
		SigningOrder:   recipient.SigningOrder,
		RemainingCount: 0,
		TenantID:       tenantID,
	})

	// Publish request completed event
	s.publisher.PublishRequestCompleted(ctx, &event.SigningRequestCompletedData{
		RequestID:    signingRequest.ID,
		TemplateID:   signingRequest.TemplateID,
		TemplateName: template.Name,
		SignedFileKey: signingRequest.OriginalFileKey,
		TenantID:     tenantID,
	})

	return &paperlessV1.SubmitSigningResponse{
		Completed: true,
		Message:   "Thank you for signing. All signatures have been collected and the document is now complete.",
	}, nil
}

// applyStagedPrefills applies prefill field values for a given stage to the PDF.
// Stage N values are applied just before recipient N begins signing.
func (s *SigningSessionService) applyStagedPrefills(ctx context.Context, signingRequest *ent.SigningRequest, template *ent.SigningTemplate, stage int) {
	// Collect fills for this stage, using template positions
	var fills []FieldFill
	for _, tmplField := range template.Fields {
		if tmplField.PrefillStage != stage {
			continue
		}
		// Find the prefill value stored in the request (match by ID or Name for backwards compatibility)
		for _, fv := range signingRequest.FieldValues {
			if fv.FieldID == tmplField.ID || fv.FieldID == tmplField.Name {
				fills = append(fills, FieldFill{
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

	if len(fills) == 0 {
		return
	}

	pdfBytes, err := s.storage.Download(ctx, signingRequest.OriginalFileKey)
	if err != nil {
		s.log.Errorf("failed to download PDF for stage-%d prefill: %v", stage, err)
		return
	}

	modifiedPDF, err := s.processor.FillFields(pdfBytes, fills, "", "")
	if err != nil {
		s.log.Errorf("failed to apply stage-%d prefill fields: %v", stage, err)
		return
	}

	if _, err := s.storage.UploadRaw(ctx, signingRequest.OriginalFileKey, modifiedPDF, "application/pdf"); err != nil {
		s.log.Errorf("failed to upload stage-%d pre-filled PDF: %v", stage, err)
	} else {
		s.log.Infof("applied %d stage-%d prefill fields to PDF", len(fills), stage)
	}
}

// sendNextSigningEmail sends a signing email to the next recipient
func (s *SigningSessionService) sendNextSigningEmail(email, name, requestName, message, token string) {
	appHost := os.Getenv("APP_HOST")
	if appHost == "" {
		appHost = "http://localhost:8080"
	}
	signingLink := fmt.Sprintf("%s/#/signing/%s", appHost, token)

	subject := fmt.Sprintf("Document signing request: %s", requestName)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
  <div style="background: #f8f9fa; border-radius: 8px; padding: 30px;">
    <h2 style="color: #333; margin-top: 0;">Document Signing Request</h2>
    <p>Hello %s,</p>
    <p>It is now your turn to sign the document: <strong>%s</strong></p>
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
  </div>
</body>
</html>`, name, requestName, formatMessage(message), signingLink, signingLink, signingLink)

	if err := s.smtp.Send(email, subject, htmlBody); err != nil {
		s.log.Errorf("failed to send signing email to %s: %v", email, err)
	}
}

func fieldTypeToSessionProto(t string) paperlessV1.SigningFieldType {
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

// extractClientIP extracts the client IP from gRPC metadata
func extractClientIP(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if ips := md.Get("x-client-ip"); len(ips) > 0 {
			return ips[0]
		}
		if ips := md.Get("x-real-ip"); len(ips) > 0 {
			return ips[0]
		}
	}
	return ""
}
