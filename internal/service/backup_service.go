package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/timestamppb"

	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/go-tangra/go-tangra-common/grpcx"

	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/category"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/document"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/documentpermission"
)

const (
	backupModule  = "paperless"
	backupVersion = "1.0"
)

type BackupService struct {
	paperlessV1.UnimplementedBackupServiceServer

	log       *log.Helper
	entClient *entCrud.EntClient[*ent.Client]
}

func NewBackupService(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *BackupService {
	return &BackupService{
		log:       ctx.NewLoggerHelper("paperless/service/backup"),
		entClient: entClient,
	}
}

type backupData struct {
	Module     string          `json:"module"`
	Version    string          `json:"version"`
	ExportedAt time.Time      `json:"exportedAt"`
	TenantID   uint32         `json:"tenantId"`
	FullBackup bool           `json:"fullBackup"`
	Data       backupEntities `json:"data"`
}

type backupEntities struct {
	Categories          []json.RawMessage `json:"categories,omitempty"`
	Documents           []json.RawMessage `json:"documents,omitempty"`
	DocumentPermissions []json.RawMessage `json:"documentPermissions,omitempty"`
}

func (s *BackupService) ExportBackup(ctx context.Context, req *paperlessV1.ExportBackupRequest) (*paperlessV1.ExportBackupResponse, error) {
	tenantID := grpcx.GetTenantIDFromContext(ctx)
	full := false

	if grpcx.IsPlatformAdmin(ctx) && req.TenantId != nil && *req.TenantId == 0 {
		full = true
		tenantID = 0
	} else if req.TenantId != nil && *req.TenantId != 0 {
		if grpcx.IsPlatformAdmin(ctx) {
			tenantID = *req.TenantId
		}
	}

	client := s.entClient.Client()
	now := time.Now()

	categories, err := s.exportCategories(ctx, client, tenantID, full)
	if err != nil {
		return nil, fmt.Errorf("export categories: %w", err)
	}
	documents, err := s.exportDocuments(ctx, client, tenantID, full)
	if err != nil {
		return nil, fmt.Errorf("export documents: %w", err)
	}
	documentPermissions, err := s.exportDocumentPermissions(ctx, client, tenantID, full)
	if err != nil {
		return nil, fmt.Errorf("export document permissions: %w", err)
	}

	backup := backupData{
		Module:     backupModule,
		Version:    backupVersion,
		ExportedAt: now,
		TenantID:   tenantID,
		FullBackup: full,
		Data: backupEntities{
			Categories:          categories,
			Documents:           documents,
			DocumentPermissions: documentPermissions,
		},
	}

	data, err := json.Marshal(backup)
	if err != nil {
		return nil, fmt.Errorf("marshal backup: %w", err)
	}

	entityCounts := map[string]int64{
		"categories":          int64(len(categories)),
		"documents":           int64(len(documents)),
		"documentPermissions": int64(len(documentPermissions)),
	}

	s.log.Infof("exported backup: module=%s tenant=%d full=%v entities=%v", backupModule, tenantID, full, entityCounts)

	return &paperlessV1.ExportBackupResponse{
		Data:         data,
		Module:       backupModule,
		Version:      backupVersion,
		ExportedAt:   timestamppb.New(now),
		TenantId:     tenantID,
		EntityCounts: entityCounts,
	}, nil
}

func (s *BackupService) ImportBackup(ctx context.Context, req *paperlessV1.ImportBackupRequest) (*paperlessV1.ImportBackupResponse, error) {
	tenantID := grpcx.GetTenantIDFromContext(ctx)
	isPlatformAdmin := grpcx.IsPlatformAdmin(ctx)
	mode := req.GetMode()

	var backup backupData
	if err := json.Unmarshal(req.GetData(), &backup); err != nil {
		return nil, fmt.Errorf("invalid backup data: %w", err)
	}

	if backup.Module != backupModule {
		return nil, fmt.Errorf("backup module mismatch: expected %s, got %s", backupModule, backup.Module)
	}
	if backup.Version != backupVersion {
		return nil, fmt.Errorf("backup version mismatch: expected %s, got %s", backupVersion, backup.Version)
	}

	// For full backups, only platform admins can restore
	if backup.FullBackup && !isPlatformAdmin {
		return nil, fmt.Errorf("only platform admins can restore full backups")
	}

	// Non-platform admins always restore to their own tenant
	if !isPlatformAdmin || !backup.FullBackup {
		tenantID = grpcx.GetTenantIDFromContext(ctx)
	} else {
		tenantID = 0 // Signal for full backup restore — each entity carries its own tenant_id
	}

	client := s.entClient.Client()
	var results []*paperlessV1.EntityImportResult
	var warnings []string

	// Import in FK dependency order
	importFuncs := []struct {
		name string
		fn   func(ctx context.Context, client *ent.Client, items []json.RawMessage, tenantID uint32, full bool, mode paperlessV1.RestoreMode) (*paperlessV1.EntityImportResult, []string)
	}{
		{"categories", s.importCategories},
		{"documents", s.importDocuments},
		{"documentPermissions", s.importDocumentPermissions},
	}

	dataMap := map[string][]json.RawMessage{
		"categories":          backup.Data.Categories,
		"documents":           backup.Data.Documents,
		"documentPermissions": backup.Data.DocumentPermissions,
	}

	for _, imp := range importFuncs {
		items := dataMap[imp.name]
		if len(items) == 0 {
			continue
		}
		result, w := imp.fn(ctx, client, items, tenantID, backup.FullBackup, mode)
		if result != nil {
			results = append(results, result)
		}
		warnings = append(warnings, w...)
	}

	s.log.Infof("imported backup: module=%s tenant=%d mode=%v results=%d warnings=%d", backupModule, tenantID, mode, len(results), len(warnings))

	return &paperlessV1.ImportBackupResponse{
		Success:  true,
		Results:  results,
		Warnings: warnings,
	}, nil
}

// topologicalSortByParentID sorts items so parents come before children.
func topologicalSortByParentID[T any](items []T, getID func(T) string, getParentID func(T) string) []T {
	idSet := make(map[string]bool, len(items))
	for _, item := range items {
		idSet[getID(item)] = true
	}

	childMap := make(map[string][]T)
	var roots []T
	for _, item := range items {
		pid := getParentID(item)
		if pid == "" || !idSet[pid] {
			roots = append(roots, item)
		} else {
			childMap[pid] = append(childMap[pid], item)
		}
	}

	result := make([]T, 0, len(items))
	var walk func([]T)
	walk = func(nodes []T) {
		for _, n := range nodes {
			result = append(result, n)
			if children, ok := childMap[getID(n)]; ok {
				walk(children)
			}
		}
	}
	walk(roots)
	return result
}

func marshalEntities[T any](entities []*T) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, 0, len(entities))
	for _, e := range entities {
		b, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, nil
}

// --- Export helpers ---

func (s *BackupService) exportCategories(ctx context.Context, client *ent.Client, tenantID uint32, full bool) ([]json.RawMessage, error) {
	query := client.Category.Query()
	if !full {
		query = query.Where(category.TenantID(tenantID))
	}
	entities, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	return marshalEntities(entities)
}

func (s *BackupService) exportDocuments(ctx context.Context, client *ent.Client, tenantID uint32, full bool) ([]json.RawMessage, error) {
	query := client.Document.Query()
	if !full {
		query = query.Where(document.TenantID(tenantID))
	}
	entities, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	return marshalEntities(entities)
}

func (s *BackupService) exportDocumentPermissions(ctx context.Context, client *ent.Client, tenantID uint32, full bool) ([]json.RawMessage, error) {
	query := client.DocumentPermission.Query()
	if !full {
		query = query.Where(documentpermission.TenantID(tenantID))
	}
	entities, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	return marshalEntities(entities)
}

// --- Import helpers ---

func (s *BackupService) importCategories(ctx context.Context, client *ent.Client, items []json.RawMessage, tenantID uint32, full bool, mode paperlessV1.RestoreMode) (*paperlessV1.EntityImportResult, []string) {
	result := &paperlessV1.EntityImportResult{EntityType: "categories", Total: int64(len(items))}
	var warnings []string

	var entities []*ent.Category
	for _, raw := range items {
		var e ent.Category
		if err := json.Unmarshal(raw, &e); err != nil {
			warnings = append(warnings, fmt.Sprintf("categories: unmarshal error: %v", err))
			result.Failed++
			continue
		}
		entities = append(entities, &e)
	}

	// Topological sort for self-referential parent_id
	sorted := topologicalSortByParentID(entities,
		func(e *ent.Category) string { return e.ID },
		func(e *ent.Category) string {
			if e.ParentID == nil {
				return ""
			}
			return *e.ParentID
		},
	)

	for _, e := range sorted {
		tid := tenantID
		if full && e.TenantID != nil {
			tid = *e.TenantID
		}

		existing, _ := client.Category.Get(ctx, e.ID)
		if existing != nil {
			if mode == paperlessV1.RestoreMode_RESTORE_MODE_SKIP {
				result.Skipped++
				continue
			}
			// Overwrite
			update := client.Category.UpdateOneID(e.ID).
				SetName(e.Name).
				SetPath(e.Path).
				SetDescription(e.Description).
				SetDepth(e.Depth).
				SetSortOrder(e.SortOrder).
				SetNillableParentID(e.ParentID).
				SetNillableCreateBy(e.CreateBy)
			_, err := update.Save(ctx)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("categories: update %s: %v", e.ID, err))
				result.Failed++
				continue
			}
			result.Updated++
		} else {
			// Create
			create := client.Category.Create().
				SetID(e.ID).
				SetNillableTenantID(&tid).
				SetName(e.Name).
				SetPath(e.Path).
				SetDescription(e.Description).
				SetDepth(e.Depth).
				SetSortOrder(e.SortOrder).
				SetNillableParentID(e.ParentID).
				SetNillableCreateBy(e.CreateBy).
				SetNillableCreateTime(e.CreateTime)
			_, err := create.Save(ctx)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("categories: create %s: %v", e.ID, err))
				result.Failed++
				continue
			}
			result.Created++
		}
	}

	return result, warnings
}

func (s *BackupService) importDocuments(ctx context.Context, client *ent.Client, items []json.RawMessage, tenantID uint32, full bool, mode paperlessV1.RestoreMode) (*paperlessV1.EntityImportResult, []string) {
	result := &paperlessV1.EntityImportResult{EntityType: "documents", Total: int64(len(items))}
	var warnings []string

	for _, raw := range items {
		var e ent.Document
		if err := json.Unmarshal(raw, &e); err != nil {
			warnings = append(warnings, fmt.Sprintf("documents: unmarshal error: %v", err))
			result.Failed++
			continue
		}

		tid := tenantID
		if full && e.TenantID != nil {
			tid = *e.TenantID
		}

		existing, _ := client.Document.Get(ctx, e.ID)
		if existing != nil {
			if mode == paperlessV1.RestoreMode_RESTORE_MODE_SKIP {
				result.Skipped++
				continue
			}
			_, err := client.Document.UpdateOneID(e.ID).
				SetNillableCategoryID(e.CategoryID).
				SetName(e.Name).
				SetDescription(e.Description).
				SetFileKey(e.FileKey).
				SetFileName(e.FileName).
				SetFileSize(e.FileSize).
				SetMimeType(e.MimeType).
				SetChecksum(e.Checksum).
				SetTags(e.Tags).
				SetStatus(e.Status).
				SetSource(e.Source).
				SetContentText(e.ContentText).
				SetExtractedMetadata(e.ExtractedMetadata).
				SetProcessingStatus(e.ProcessingStatus).
				SetNillableCreateBy(e.CreateBy).
				SetNillableUpdateBy(e.UpdateBy).
				Save(ctx)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("documents: update %s: %v", e.ID, err))
				result.Failed++
				continue
			}
			result.Updated++
		} else {
			_, err := client.Document.Create().
				SetID(e.ID).
				SetNillableTenantID(&tid).
				SetNillableCategoryID(e.CategoryID).
				SetName(e.Name).
				SetDescription(e.Description).
				SetFileKey(e.FileKey).
				SetFileName(e.FileName).
				SetFileSize(e.FileSize).
				SetMimeType(e.MimeType).
				SetChecksum(e.Checksum).
				SetTags(e.Tags).
				SetStatus(e.Status).
				SetSource(e.Source).
				SetContentText(e.ContentText).
				SetExtractedMetadata(e.ExtractedMetadata).
				SetProcessingStatus(e.ProcessingStatus).
				SetNillableCreateBy(e.CreateBy).
				SetNillableUpdateBy(e.UpdateBy).
				SetNillableCreateTime(e.CreateTime).
				Save(ctx)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("documents: create %s: %v", e.ID, err))
				result.Failed++
				continue
			}
			result.Created++
		}
	}

	return result, warnings
}

func (s *BackupService) importDocumentPermissions(ctx context.Context, client *ent.Client, items []json.RawMessage, tenantID uint32, full bool, mode paperlessV1.RestoreMode) (*paperlessV1.EntityImportResult, []string) {
	result := &paperlessV1.EntityImportResult{EntityType: "documentPermissions", Total: int64(len(items))}
	var warnings []string

	for _, raw := range items {
		var e ent.DocumentPermission
		if err := json.Unmarshal(raw, &e); err != nil {
			warnings = append(warnings, fmt.Sprintf("documentPermissions: unmarshal error: %v", err))
			result.Failed++
			continue
		}

		tid := tenantID
		if full && e.TenantID != nil {
			tid = *e.TenantID
		}

		existing, _ := client.DocumentPermission.Get(ctx, e.ID)
		if existing != nil {
			if mode == paperlessV1.RestoreMode_RESTORE_MODE_SKIP {
				result.Skipped++
				continue
			}
			_, err := client.DocumentPermission.UpdateOneID(e.ID).
				SetResourceType(e.ResourceType).
				SetResourceID(e.ResourceID).
				SetRelation(e.Relation).
				SetSubjectType(e.SubjectType).
				SetSubjectID(e.SubjectID).
				SetNillableGrantedBy(e.GrantedBy).
				SetNillableExpiresAt(e.ExpiresAt).
				Save(ctx)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("documentPermissions: update %d: %v", e.ID, err))
				result.Failed++
				continue
			}
			result.Updated++
		} else {
			_, err := client.DocumentPermission.Create().
				SetNillableTenantID(&tid).
				SetResourceType(e.ResourceType).
				SetResourceID(e.ResourceID).
				SetRelation(e.Relation).
				SetSubjectType(e.SubjectType).
				SetSubjectID(e.SubjectID).
				SetNillableGrantedBy(e.GrantedBy).
				SetNillableExpiresAt(e.ExpiresAt).
				SetNillableCreateTime(e.CreateTime).
				Save(ctx)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("documentPermissions: create %d: %v", e.ID, err))
				result.Failed++
				continue
			}
			result.Created++
		}
	}

	return result, warnings
}
