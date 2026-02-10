package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/go-tangra/go-tangra-paperless/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/internal/data"

	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
)

type PermissionService struct {
	paperlessV1.UnimplementedPaperlessPermissionServiceServer

	log      *log.Helper
	permRepo *data.PermissionRepo
	engine   *authz.Engine
}

func NewPermissionService(
	ctx *bootstrap.Context,
	permRepo *data.PermissionRepo,
	engine *authz.Engine,
) *PermissionService {
	return &PermissionService{
		log:      ctx.NewLoggerHelper("paperless/service/permission"),
		permRepo: permRepo,
		engine:   engine,
	}
}

// GrantAccess grants access to a resource
func (s *PermissionService) GrantAccess(ctx context.Context, req *paperlessV1.GrantAccessRequest) (*paperlessV1.GrantAccessResponse, error) {
	tenantID := getTenantIDFromContext(ctx)
	grantedBy := getUserIDAsUint32(ctx)

	permission, err := s.permRepo.Create(ctx, tenantID,
		req.ResourceType.String(),
		req.ResourceId,
		req.Relation.String(),
		req.SubjectType.String(),
		req.SubjectId,
		grantedBy,
		nil, // expiresAt - simplified for now
	)
	if err != nil {
		return nil, err
	}

	return &paperlessV1.GrantAccessResponse{
		Permission: s.permRepo.ToProto(permission),
	}, nil
}

// RevokeAccess revokes access from a resource
func (s *PermissionService) RevokeAccess(ctx context.Context, req *paperlessV1.RevokeAccessRequest) (*emptypb.Empty, error) {
	tenantID := getTenantIDFromContext(ctx)

	var relation *string
	if req.Relation != nil && *req.Relation != paperlessV1.Relation_RELATION_UNSPECIFIED {
		r := req.Relation.String()
		relation = &r
	}

	err := s.permRepo.Delete(ctx, tenantID,
		req.ResourceType.String(),
		req.ResourceId,
		relation,
		req.SubjectType.String(),
		req.SubjectId,
	)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// ListPermissions lists permissions
func (s *PermissionService) ListPermissions(ctx context.Context, req *paperlessV1.ListPermissionsRequest) (*paperlessV1.ListPermissionsResponse, error) {
	tenantID := getTenantIDFromContext(ctx)

	page := uint32(1)
	if req.Page != nil {
		page = *req.Page
	}
	pageSize := uint32(20)
	if req.PageSize != nil {
		pageSize = *req.PageSize
	}

	var resourceType, subjectType *string
	if req.ResourceType != nil && *req.ResourceType != paperlessV1.ResourceType_RESOURCE_TYPE_UNSPECIFIED {
		rt := req.ResourceType.String()
		resourceType = &rt
	}
	if req.SubjectType != nil && *req.SubjectType != paperlessV1.SubjectType_SUBJECT_TYPE_UNSPECIFIED {
		st := req.SubjectType.String()
		subjectType = &st
	}

	permissions, total, err := s.permRepo.List(ctx, tenantID, resourceType, req.ResourceId, subjectType, req.SubjectId, page, pageSize)
	if err != nil {
		return nil, err
	}

	protoPermissions := make([]*paperlessV1.PermissionTuple, 0, len(permissions))
	for _, perm := range permissions {
		protoPermissions = append(protoPermissions, s.permRepo.ToProto(perm))
	}

	return &paperlessV1.ListPermissionsResponse{
		Permissions: protoPermissions,
		Total:       uint32(total),
	}, nil
}

// CheckAccess checks if a subject has access to a resource using the authz engine
func (s *PermissionService) CheckAccess(ctx context.Context, req *paperlessV1.CheckAccessRequest) (*paperlessV1.CheckAccessResponse, error) {
	tenantID := getTenantIDFromContext(ctx)

	result := s.engine.Check(ctx, authz.CheckContext{
		TenantID:     tenantID,
		UserID:       req.UserId,
		ResourceType: authz.ResourceType(req.ResourceType.String()),
		ResourceID:   req.ResourceId,
		Permission:   authz.Permission(req.Permission.String()),
	})

	var reason *string
	if !result.Allowed {
		reason = &result.Reason
	}

	return &paperlessV1.CheckAccessResponse{
		Allowed: result.Allowed,
		Reason:  reason,
	}, nil
}

// ListAccessibleResources lists resources accessible by a user using the authz engine
func (s *PermissionService) ListAccessibleResources(ctx context.Context, req *paperlessV1.ListAccessibleResourcesRequest) (*paperlessV1.ListAccessibleResourcesResponse, error) {
	tenantID := getTenantIDFromContext(ctx)

	resourceIDs, err := s.engine.ListAccessibleResources(ctx, tenantID, req.UserId, authz.ResourceType(req.ResourceType.String()), authz.PermissionRead)
	if err != nil {
		return nil, err
	}

	total := uint32(len(resourceIDs))

	// Apply pagination
	page := uint32(1)
	if req.Page != nil {
		page = *req.Page
	}
	pageSize := uint32(20)
	if req.PageSize != nil {
		pageSize = *req.PageSize
	}

	if page > 0 && pageSize > 0 {
		start := int((page - 1) * pageSize)
		end := start + int(pageSize)
		if start >= len(resourceIDs) {
			resourceIDs = []string{}
		} else if end > len(resourceIDs) {
			resourceIDs = resourceIDs[start:]
		} else {
			resourceIDs = resourceIDs[start:end]
		}
	}

	return &paperlessV1.ListAccessibleResourcesResponse{
		ResourceIds: resourceIDs,
		Total:       total,
	}, nil
}

// GetEffectivePermissions gets effective permissions for a user on a resource using the authz engine
func (s *PermissionService) GetEffectivePermissions(ctx context.Context, req *paperlessV1.GetEffectivePermissionsRequest) (*paperlessV1.GetEffectivePermissionsResponse, error) {
	tenantID := getTenantIDFromContext(ctx)

	permissions, highestRelation := s.engine.GetEffectivePermissions(ctx, authz.CheckContext{
		TenantID:     tenantID,
		UserID:       req.UserId,
		ResourceType: authz.ResourceType(req.ResourceType.String()),
		ResourceID:   req.ResourceId,
	})

	// Convert authz permissions to proto permissions
	protoPermissions := make([]paperlessV1.Permission, 0, len(permissions))
	for _, p := range permissions {
		if pv, ok := paperlessV1.Permission_value[string(p)]; ok {
			protoPermissions = append(protoPermissions, paperlessV1.Permission(pv))
		}
	}

	return &paperlessV1.GetEffectivePermissionsResponse{
		Permissions:     protoPermissions,
		HighestRelation: paperlessV1.Relation(paperlessV1.Relation_value[string(highestRelation)]),
	}, nil
}
