package service

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/timestamppb"

	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/go-tangra/go-tangra-common/backup"
	"github.com/go-tangra/go-tangra-common/grpcx"

	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/category"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/document"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/documentpermission"
)

const (
	backupModule        = "paperless"
	backupSchemaVersion = 1
)

// Migrations registry — add entries here when schema changes.
var migrations = backup.NewMigrationRegistry(backupModule)

// Register migrations in init. Example for future use:
//
//	func init() {
//	    migrations.Register(1, func(entities map[string]json.RawMessage) error {
//	        return backup.MigrateAddField(entities, "documents", "newField", "")
//	    })
//	}

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

// ExportBackup exports all paperless entities as a gzipped archive.
func (s *BackupService) ExportBackup(ctx context.Context, req *paperlessV1.ExportBackupRequest) (*paperlessV1.ExportBackupResponse, error) {
	tenantID := grpcx.GetTenantIDFromContext(ctx)
	full := false

	if grpcx.IsPlatformAdmin(ctx) && req.TenantId != nil && *req.TenantId == 0 {
		full = true
		tenantID = 0
	} else if req.TenantId != nil && *req.TenantId != 0 && grpcx.IsPlatformAdmin(ctx) {
		tenantID = *req.TenantId
	}

	client := s.entClient.Client()
	a := backup.NewArchive(backupModule, backupSchemaVersion, tenantID, full)

	// Export categories
	catQuery := client.Category.Query()
	if !full {
		catQuery = catQuery.Where(category.TenantID(tenantID))
	}
	categories, err := catQuery.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("export categories: %w", err)
	}
	if err := backup.SetEntities(a, "categories", categories); err != nil {
		return nil, fmt.Errorf("set categories: %w", err)
	}

	// Export documents
	docQuery := client.Document.Query()
	if !full {
		docQuery = docQuery.Where(document.TenantID(tenantID))
	}
	documents, err := docQuery.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("export documents: %w", err)
	}
	if err := backup.SetEntities(a, "documents", documents); err != nil {
		return nil, fmt.Errorf("set documents: %w", err)
	}

	// Export document permissions
	permQuery := client.DocumentPermission.Query()
	if !full {
		permQuery = permQuery.Where(documentpermission.TenantID(tenantID))
	}
	permissions, err := permQuery.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("export document permissions: %w", err)
	}
	if err := backup.SetEntities(a, "documentPermissions", permissions); err != nil {
		return nil, fmt.Errorf("set document permissions: %w", err)
	}

	// Pack (JSON + gzip)
	data, err := backup.Pack(a)
	if err != nil {
		return nil, fmt.Errorf("pack backup: %w", err)
	}

	s.log.Infof("exported backup: module=%s tenant=%d full=%v entities=%v", backupModule, tenantID, full, a.Manifest.EntityCounts)

	return &paperlessV1.ExportBackupResponse{
		Data:          data,
		Module:        backupModule,
		Version:       fmt.Sprintf("%d", backupSchemaVersion),
		ExportedAt:    timestamppb.New(a.Manifest.ExportedAt),
		TenantId:      tenantID,
		EntityCounts:  a.Manifest.EntityCounts,
		SchemaVersion: int32(backupSchemaVersion),
	}, nil
}

// ImportBackup restores paperless entities from a gzipped archive.
func (s *BackupService) ImportBackup(ctx context.Context, req *paperlessV1.ImportBackupRequest) (*paperlessV1.ImportBackupResponse, error) {
	tenantID := grpcx.GetTenantIDFromContext(ctx)
	isPlatformAdmin := grpcx.IsPlatformAdmin(ctx)
	mode := mapRestoreMode(req.GetMode())

	// Unpack
	a, err := backup.Unpack(req.GetData())
	if err != nil {
		return nil, fmt.Errorf("unpack backup: %w", err)
	}

	// Validate
	if err := backup.Validate(a, backupModule, backupSchemaVersion); err != nil {
		return nil, err
	}

	// Full backups require platform admin
	if a.Manifest.FullBackup && !isPlatformAdmin {
		return nil, fmt.Errorf("only platform admins can restore full backups")
	}

	// Run migrations if needed
	sourceVersion := a.Manifest.SchemaVersion
	applied, err := migrations.RunMigrations(a, backupSchemaVersion)
	if err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	// Determine restore tenant
	if !isPlatformAdmin || !a.Manifest.FullBackup {
		tenantID = grpcx.GetTenantIDFromContext(ctx)
	} else {
		tenantID = 0
	}

	client := s.entClient.Client()
	result := backup.NewRestoreResult(sourceVersion, backupSchemaVersion, applied)

	// Import categories (with topological sort for parent_id)
	s.importCategories(ctx, client, a, tenantID, a.Manifest.FullBackup, mode, result)

	// Import documents
	s.importDocuments(ctx, client, a, tenantID, a.Manifest.FullBackup, mode, result)

	// Import document permissions
	s.importDocumentPermissions(ctx, client, a, tenantID, a.Manifest.FullBackup, mode, result)

	s.log.Infof("imported backup: module=%s tenant=%d mode=%v migrations=%d results=%d",
		backupModule, tenantID, mode, applied, len(result.Results))

	// Convert to proto response
	protoResults := make([]*paperlessV1.EntityImportResult, len(result.Results))
	for i, r := range result.Results {
		protoResults[i] = &paperlessV1.EntityImportResult{
			EntityType: r.EntityType,
			Total:      r.Total,
			Created:    r.Created,
			Updated:    r.Updated,
			Skipped:    r.Skipped,
			Failed:     r.Failed,
		}
	}

	return &paperlessV1.ImportBackupResponse{
		Success:           result.Success,
		Results:           protoResults,
		Warnings:          result.Warnings,
		SourceVersion:     int32(result.SourceVersion),
		TargetVersion:     int32(result.TargetVersion),
		MigrationsApplied: int32(result.MigrationsApplied),
	}, nil
}

func mapRestoreMode(m paperlessV1.RestoreMode) backup.RestoreMode {
	if m == paperlessV1.RestoreMode_RESTORE_MODE_OVERWRITE {
		return backup.RestoreModeOverwrite
	}
	return backup.RestoreModeSkip
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

	sorted := make([]T, 0, len(items))
	var walk func([]T)
	walk = func(nodes []T) {
		for _, n := range nodes {
			sorted = append(sorted, n)
			if children, ok := childMap[getID(n)]; ok {
				walk(children)
			}
		}
	}
	walk(roots)
	return sorted
}

// --- Import helpers ---

func (s *BackupService) importCategories(ctx context.Context, client *ent.Client, a *backup.Archive, tenantID uint32, full bool, mode backup.RestoreMode, result *backup.RestoreResult) {
	categories, err := backup.GetEntities[ent.Category](a, "categories")
	if err != nil {
		result.AddWarning(fmt.Sprintf("categories: unmarshal error: %v", err))
		return
	}
	if len(categories) == 0 {
		return
	}

	er := backup.EntityResult{EntityType: "categories", Total: int64(len(categories))}

	// Topological sort for self-referential parent_id
	sorted := topologicalSortByParentID(categories,
		func(e ent.Category) string { return e.ID },
		func(e ent.Category) string {
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

		existing, getErr := client.Category.Get(ctx, e.ID)
		if getErr != nil && !ent.IsNotFound(getErr) {
			result.AddWarning(fmt.Sprintf("categories: lookup %s: %v", e.ID, getErr))
			er.Failed++
			continue
		}
		if existing != nil {
			if mode == backup.RestoreModeSkip {
				er.Skipped++
				continue
			}
			_, err := client.Category.UpdateOneID(e.ID).
				SetName(e.Name).
				SetPath(e.Path).
				SetDescription(e.Description).
				SetDepth(e.Depth).
				SetSortOrder(e.SortOrder).
				SetNillableParentID(e.ParentID).
				SetNillableCreateBy(e.CreateBy).
				Save(ctx)
			if err != nil {
				result.AddWarning(fmt.Sprintf("categories: update %s: %v", e.ID, err))
				er.Failed++
				continue
			}
			er.Updated++
		} else {
			_, err := client.Category.Create().
				SetID(e.ID).
				SetNillableTenantID(&tid).
				SetName(e.Name).
				SetPath(e.Path).
				SetDescription(e.Description).
				SetDepth(e.Depth).
				SetSortOrder(e.SortOrder).
				SetNillableParentID(e.ParentID).
				SetNillableCreateBy(e.CreateBy).
				SetNillableCreateTime(e.CreateTime).
				Save(ctx)
			if err != nil {
				result.AddWarning(fmt.Sprintf("categories: create %s: %v", e.ID, err))
				er.Failed++
				continue
			}
			er.Created++
		}
	}

	result.AddResult(er)
}

func (s *BackupService) importDocuments(ctx context.Context, client *ent.Client, a *backup.Archive, tenantID uint32, full bool, mode backup.RestoreMode, result *backup.RestoreResult) {
	documents, err := backup.GetEntities[ent.Document](a, "documents")
	if err != nil {
		result.AddWarning(fmt.Sprintf("documents: unmarshal error: %v", err))
		return
	}
	if len(documents) == 0 {
		return
	}

	er := backup.EntityResult{EntityType: "documents", Total: int64(len(documents))}

	for _, e := range documents {
		tid := tenantID
		if full && e.TenantID != nil {
			tid = *e.TenantID
		}

		existing, getErr := client.Document.Get(ctx, e.ID)
		if getErr != nil && !ent.IsNotFound(getErr) {
			result.AddWarning(fmt.Sprintf("documents: lookup %s: %v", e.ID, getErr))
			er.Failed++
			continue
		}
		if existing != nil {
			if mode == backup.RestoreModeSkip {
				er.Skipped++
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
				result.AddWarning(fmt.Sprintf("documents: update %s: %v", e.ID, err))
				er.Failed++
				continue
			}
			er.Updated++
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
				result.AddWarning(fmt.Sprintf("documents: create %s: %v", e.ID, err))
				er.Failed++
				continue
			}
			er.Created++
		}
	}

	result.AddResult(er)
}

func (s *BackupService) importDocumentPermissions(ctx context.Context, client *ent.Client, a *backup.Archive, tenantID uint32, full bool, mode backup.RestoreMode, result *backup.RestoreResult) {
	permissions, err := backup.GetEntities[ent.DocumentPermission](a, "documentPermissions")
	if err != nil {
		result.AddWarning(fmt.Sprintf("documentPermissions: unmarshal error: %v", err))
		return
	}
	if len(permissions) == 0 {
		return
	}

	er := backup.EntityResult{EntityType: "documentPermissions", Total: int64(len(permissions))}

	for _, e := range permissions {
		tid := tenantID
		if full && e.TenantID != nil {
			tid = *e.TenantID
		}

		existing, getErr := client.DocumentPermission.Get(ctx, e.ID)
		if getErr != nil && !ent.IsNotFound(getErr) {
			result.AddWarning(fmt.Sprintf("documentPermissions: lookup %d: %v", e.ID, getErr))
			er.Failed++
			continue
		}
		if existing != nil {
			if mode == backup.RestoreModeSkip {
				er.Skipped++
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
				result.AddWarning(fmt.Sprintf("documentPermissions: update %d: %v", e.ID, err))
				er.Failed++
				continue
			}
			er.Updated++
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
				result.AddWarning(fmt.Sprintf("documentPermissions: create %d: %v", e.ID, err))
				er.Failed++
				continue
			}
			er.Created++
		}
	}

	result.AddResult(er)
}
