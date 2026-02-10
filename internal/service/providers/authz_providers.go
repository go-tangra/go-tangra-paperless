package providers

import (
	"context"

	"github.com/go-tangra/go-tangra-common/grpcx"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	"github.com/go-tangra/go-tangra-paperless/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/internal/data"
)

// ProvideResourceLookup creates a ResourceLookup from repositories
func ProvideResourceLookup(categoryRepo *data.CategoryRepo, documentRepo *data.DocumentRepo) authz.ResourceLookup {
	return &resourceLookupImpl{
		categoryRepo: categoryRepo,
		documentRepo: documentRepo,
	}
}

// ProvidePermissionStore creates a PermissionStore from the permission repo
func ProvidePermissionStore(permRepo *data.PermissionRepo) authz.PermissionStore {
	return &permissionStoreAdapter{permRepo: permRepo}
}

// ProvideAuthzEngine creates the authorization engine
func ProvideAuthzEngine(store authz.PermissionStore, lookup authz.ResourceLookup, ctx *bootstrap.Context) *authz.Engine {
	return authz.NewEngine(store, lookup, ctx.GetLogger())
}

// ProvideAuthzChecker creates the authorization checker
func ProvideAuthzChecker(engine *authz.Engine) *authz.Checker {
	return authz.NewChecker(engine)
}

// resourceLookupImpl implements authz.ResourceLookup
type resourceLookupImpl struct {
	categoryRepo *data.CategoryRepo
	documentRepo *data.DocumentRepo
}

func (r *resourceLookupImpl) GetCategoryParentID(ctx context.Context, tenantID uint32, categoryID string) (*string, error) {
	return r.categoryRepo.GetCategoryParentID(ctx, tenantID, categoryID)
}

func (r *resourceLookupImpl) GetDocumentCategoryID(ctx context.Context, tenantID uint32, documentID string) (*string, error) {
	return r.documentRepo.GetDocumentCategoryID(ctx, tenantID, documentID)
}

func (r *resourceLookupImpl) GetUserRoleIDs(ctx context.Context, tenantID uint32, userID string) ([]string, error) {
	return grpcx.GetRolesFromContext(ctx), nil
}

// permissionStoreAdapter adapts PermissionRepo to authz.PermissionStore
type permissionStoreAdapter struct {
	permRepo *data.PermissionRepo
}

func (a *permissionStoreAdapter) GetDirectPermissions(ctx context.Context, tenantID uint32, resourceType authz.ResourceType, resourceID string) ([]authz.PermissionTuple, error) {
	return a.permRepo.GetDirectPermissions(ctx, tenantID, resourceType, resourceID)
}

func (a *permissionStoreAdapter) GetSubjectPermissions(ctx context.Context, tenantID uint32, subjectType authz.SubjectType, subjectID string) ([]authz.PermissionTuple, error) {
	return a.permRepo.GetSubjectPermissions(ctx, tenantID, subjectType, subjectID)
}

func (a *permissionStoreAdapter) HasPermission(ctx context.Context, tenantID uint32, resourceType authz.ResourceType, resourceID string, subjectType authz.SubjectType, subjectID string) (*authz.PermissionTuple, error) {
	return a.permRepo.HasAuthzPermission(ctx, tenantID, resourceType, resourceID, subjectType, subjectID)
}

func (a *permissionStoreAdapter) CreatePermission(ctx context.Context, tuple authz.PermissionTuple) (*authz.PermissionTuple, error) {
	return a.permRepo.CreateAuthzPermission(ctx, tuple)
}

func (a *permissionStoreAdapter) DeletePermission(ctx context.Context, tenantID uint32, resourceType authz.ResourceType, resourceID string, relation *authz.Relation, subjectType authz.SubjectType, subjectID string) error {
	return a.permRepo.DeleteAuthzPermission(ctx, tenantID, resourceType, resourceID, relation, subjectType, subjectID)
}

func (a *permissionStoreAdapter) ListResourcesBySubject(ctx context.Context, tenantID uint32, subjectType authz.SubjectType, subjectID string, resourceType authz.ResourceType) ([]string, error) {
	return a.permRepo.ListResourcesBySubject(ctx, tenantID, subjectType, subjectID, resourceType)
}
