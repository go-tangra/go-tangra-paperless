package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/go-tangra/go-tangra-paperless/internal/data"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/schema"

	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
)

type SigningTemplateService struct {
	paperlessV1.UnimplementedPaperlessSigningTemplateServiceServer

	log          *log.Helper
	templateRepo *data.SigningTemplateRepo
	storage      *data.StorageClient
	processor    *PDFProcessor
}

func NewSigningTemplateService(
	ctx *bootstrap.Context,
	templateRepo *data.SigningTemplateRepo,
	storage *data.StorageClient,
	processor *PDFProcessor,
) *SigningTemplateService {
	return &SigningTemplateService{
		log:          ctx.NewLoggerHelper("paperless/service/signing-template"),
		templateRepo: templateRepo,
		storage:      storage,
		processor:    processor,
	}
}

// CreateSigningTemplate creates a new signing template from an uploaded PDF
func (s *SigningTemplateService) CreateSigningTemplate(ctx context.Context, req *paperlessV1.CreateSigningTemplateRequest) (*paperlessV1.CreateSigningTemplateResponse, error) {
	tenantID := getTenantIDFromContext(ctx)
	createdBy := getUserIDAsUint32(ctx)

	// Validate it's a PDF
	mimeType := http.DetectContentType(req.FileContent)
	if mimeType != "application/pdf" {
		return nil, paperlessV1.ErrorInvalidTemplatePdf("file must be a PDF document")
	}

	// Generate template ID for storage path
	templateID := uuid.New().String()

	// Upload PDF to S3
	storageKey := fmt.Sprintf("%d/signing-templates/%s/%s", tenantID, templateID, req.FileName)
	uploadResult, err := s.storage.UploadRaw(ctx, storageKey, req.FileContent, "application/pdf")
	if err != nil {
		s.log.Errorf("failed to upload template PDF: %v", err)
		return nil, paperlessV1.ErrorStorageOperationError("failed to upload template PDF")
	}

	// Create template record with empty fields (fields are added via the visual builder)
	entity, err := s.templateRepo.Create(ctx, tenantID, req.Name, req.Description, storageKey, req.FileName, uploadResult.Size, nil, createdBy)
	if err != nil {
		// Clean up uploaded file
		if delErr := s.storage.Delete(ctx, storageKey); delErr != nil {
			s.log.Warnf("failed to clean up uploaded file: %v", delErr)
		}
		return nil, err
	}

	return &paperlessV1.CreateSigningTemplateResponse{
		Template: s.templateRepo.ToProto(entity),
	}, nil
}

// GetSigningTemplate gets a signing template by ID
func (s *SigningTemplateService) GetSigningTemplate(ctx context.Context, req *paperlessV1.GetSigningTemplateRequest) (*paperlessV1.GetSigningTemplateResponse, error) {
	entity, err := s.templateRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
	}

	return &paperlessV1.GetSigningTemplateResponse{
		Template: s.templateRepo.ToProto(entity),
	}, nil
}

// ListSigningTemplates lists signing templates
func (s *SigningTemplateService) ListSigningTemplates(ctx context.Context, req *paperlessV1.ListSigningTemplatesRequest) (*paperlessV1.ListSigningTemplatesResponse, error) {
	tenantID := getTenantIDFromContext(ctx)

	page := uint32(1)
	if req.Page != nil {
		page = *req.Page
	}
	pageSize := uint32(20)
	if req.PageSize != nil {
		pageSize = *req.PageSize
	}

	entities, total, err := s.templateRepo.List(ctx, tenantID, req.NameFilter, page, pageSize)
	if err != nil {
		return nil, err
	}

	templates := make([]*paperlessV1.SigningTemplate, 0, len(entities))
	for _, entity := range entities {
		templates = append(templates, s.templateRepo.ToProto(entity))
	}

	return &paperlessV1.ListSigningTemplatesResponse{
		Templates: templates,
		Total:     uint32(total),
	}, nil
}

// UpdateSigningTemplate updates a signing template
func (s *SigningTemplateService) UpdateSigningTemplate(ctx context.Context, req *paperlessV1.UpdateSigningTemplateRequest) (*paperlessV1.UpdateSigningTemplateResponse, error) {
	updatedBy := getUserIDAsUint32(ctx)

	entity, err := s.templateRepo.Update(ctx, req.Id, req.Name, req.Description, updatedBy)
	if err != nil {
		return nil, err
	}

	return &paperlessV1.UpdateSigningTemplateResponse{
		Template: s.templateRepo.ToProto(entity),
	}, nil
}

// DeleteSigningTemplate deletes a signing template
func (s *SigningTemplateService) DeleteSigningTemplate(ctx context.Context, req *paperlessV1.DeleteSigningTemplateRequest) (*emptypb.Empty, error) {
	// Get template to retrieve file key
	entity, err := s.templateRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
	}

	// Delete from database
	if err := s.templateRepo.Delete(ctx, req.Id); err != nil {
		return nil, err
	}

	// Delete from storage
	if err := s.storage.Delete(ctx, entity.FileKey); err != nil {
		s.log.Warnf("failed to delete template file from storage: %v", err)
	}

	return &emptypb.Empty{}, nil
}

// UpdateTemplateFields updates the field definitions of a signing template (from the visual builder)
func (s *SigningTemplateService) UpdateTemplateFields(ctx context.Context, req *paperlessV1.UpdateTemplateFieldsRequest) (*paperlessV1.UpdateTemplateFieldsResponse, error) {
	updatedBy := getUserIDAsUint32(ctx)

	// Verify template exists
	entity, err := s.templateRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
	}

	// Convert proto fields to schema format
	schemaFields := make([]schema.SigningTemplateFieldDef, 0, len(req.Fields))
	for _, f := range req.Fields {
		schemaFields = append(schemaFields, schema.SigningTemplateFieldDef{
			ID:             f.Id,
			Name:           f.Name,
			Type:           protoToFieldType(f.Type),
			Required:       f.Required,
			PageNumber:     int(f.PageNumber),
			XPercent:       f.XPercent,
			YPercent:       f.YPercent,
			WidthPercent:   f.WidthPercent,
			HeightPercent:  f.HeightPercent,
			PrefillStage:   int(f.PrefillStage),
			RecipientIndex: int(f.RecipientIndex),
		})
	}

	updated, err := s.templateRepo.UpdateFields(ctx, req.Id, schemaFields, updatedBy)
	if err != nil {
		return nil, err
	}

	return &paperlessV1.UpdateTemplateFieldsResponse{
		Template: s.templateRepo.ToProto(updated),
	}, nil
}

// GetTemplatePdfUrl returns a proxy URL for viewing the template PDF
func (s *SigningTemplateService) GetTemplatePdfUrl(ctx context.Context, req *paperlessV1.GetTemplatePdfUrlRequest) (*paperlessV1.GetTemplatePdfUrlResponse, error) {
	entity, err := s.templateRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
	}

	// Generate a time-limited HMAC token for the proxy endpoint
	expiresAt := time.Now().Add(1 * time.Hour)
	token := GenerateDownloadToken(req.Id, expiresAt)
	url := fmt.Sprintf("/public/v1/signing/templates/%s/pdf?token=%s&expires=%d", req.Id, token, expiresAt.Unix())

	return &paperlessV1.GetTemplatePdfUrlResponse{
		Url: url,
	}, nil
}

// GetTemplatePdfBytes retrieves the raw PDF bytes for a signing template
func (s *SigningTemplateService) GetTemplatePdfBytes(ctx context.Context, templateID string) ([]byte, string, error) {
	entity, err := s.templateRepo.GetByID(ctx, templateID)
	if err != nil {
		return nil, "", err
	}
	if entity == nil {
		return nil, "", paperlessV1.ErrorSigningTemplateNotFound("signing template not found")
	}

	pdfBytes, err := s.storage.Download(ctx, entity.FileKey)
	if err != nil {
		s.log.Errorf("failed to download template PDF: %v", err)
		return nil, "", paperlessV1.ErrorStorageOperationError("failed to download template PDF")
	}

	return pdfBytes, entity.FileName, nil
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
