package authz

import (
	"context"
	"fmt"
)

// Checker provides a simplified interface for permission checks
type Checker struct {
	engine *Engine
}

// NewChecker creates a new permission checker
func NewChecker(engine *Engine) *Checker {
	return &Checker{engine: engine}
}

// CanRead checks if a user can read a resource
func (c *Checker) CanRead(ctx context.Context, tenantID uint32, userID string, resourceType ResourceType, resourceID string) error {
	result := c.engine.Check(ctx, CheckContext{
		TenantID:     tenantID,
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Permission:   PermissionRead,
	})
	if !result.Allowed {
		return fmt.Errorf("access denied: %s", result.Reason)
	}
	return nil
}

// CanWrite checks if a user can write to a resource
func (c *Checker) CanWrite(ctx context.Context, tenantID uint32, userID string, resourceType ResourceType, resourceID string) error {
	result := c.engine.Check(ctx, CheckContext{
		TenantID:     tenantID,
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Permission:   PermissionWrite,
	})
	if !result.Allowed {
		return fmt.Errorf("access denied: %s", result.Reason)
	}
	return nil
}

// CanDelete checks if a user can delete a resource
func (c *Checker) CanDelete(ctx context.Context, tenantID uint32, userID string, resourceType ResourceType, resourceID string) error {
	result := c.engine.Check(ctx, CheckContext{
		TenantID:     tenantID,
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Permission:   PermissionDelete,
	})
	if !result.Allowed {
		return fmt.Errorf("access denied: %s", result.Reason)
	}
	return nil
}

// CanShare checks if a user can share a resource
func (c *Checker) CanShare(ctx context.Context, tenantID uint32, userID string, resourceType ResourceType, resourceID string) error {
	result := c.engine.Check(ctx, CheckContext{
		TenantID:     tenantID,
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Permission:   PermissionShare,
	})
	if !result.Allowed {
		return fmt.Errorf("access denied: %s", result.Reason)
	}
	return nil
}

// CanDownload checks if a user can download a resource
func (c *Checker) CanDownload(ctx context.Context, tenantID uint32, userID string, resourceType ResourceType, resourceID string) error {
	result := c.engine.Check(ctx, CheckContext{
		TenantID:     tenantID,
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Permission:   PermissionDownload,
	})
	if !result.Allowed {
		return fmt.Errorf("access denied: %s", result.Reason)
	}
	return nil
}

// CheckPermission checks if a user has a specific permission on a resource
func (c *Checker) CheckPermission(ctx context.Context, tenantID uint32, userID string, resourceType ResourceType, resourceID string, permission Permission) (bool, string) {
	result := c.engine.Check(ctx, CheckContext{
		TenantID:     tenantID,
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Permission:   permission,
	})
	return result.Allowed, result.Reason
}

// RequirePermission checks if a user has a specific permission and returns an error if not
func (c *Checker) RequirePermission(ctx context.Context, tenantID uint32, userID string, resourceType ResourceType, resourceID string, permission Permission) error {
	allowed, reason := c.CheckPermission(ctx, tenantID, userID, resourceType, resourceID, permission)
	if !allowed {
		return fmt.Errorf("access denied: %s", reason)
	}
	return nil
}

// CanReadCategory is a convenience method for category read checks
func (c *Checker) CanReadCategory(ctx context.Context, tenantID uint32, userID string, categoryID string) error {
	return c.CanRead(ctx, tenantID, userID, ResourceTypeCategory, categoryID)
}

// CanWriteCategory is a convenience method for category write checks
func (c *Checker) CanWriteCategory(ctx context.Context, tenantID uint32, userID string, categoryID string) error {
	return c.CanWrite(ctx, tenantID, userID, ResourceTypeCategory, categoryID)
}

// CanDeleteCategory is a convenience method for category delete checks
func (c *Checker) CanDeleteCategory(ctx context.Context, tenantID uint32, userID string, categoryID string) error {
	return c.CanDelete(ctx, tenantID, userID, ResourceTypeCategory, categoryID)
}

// CanShareCategory is a convenience method for category share checks
func (c *Checker) CanShareCategory(ctx context.Context, tenantID uint32, userID string, categoryID string) error {
	return c.CanShare(ctx, tenantID, userID, ResourceTypeCategory, categoryID)
}

// CanReadDocument is a convenience method for document read checks
func (c *Checker) CanReadDocument(ctx context.Context, tenantID uint32, userID string, documentID string) error {
	return c.CanRead(ctx, tenantID, userID, ResourceTypeDocument, documentID)
}

// CanWriteDocument is a convenience method for document write checks
func (c *Checker) CanWriteDocument(ctx context.Context, tenantID uint32, userID string, documentID string) error {
	return c.CanWrite(ctx, tenantID, userID, ResourceTypeDocument, documentID)
}

// CanDeleteDocument is a convenience method for document delete checks
func (c *Checker) CanDeleteDocument(ctx context.Context, tenantID uint32, userID string, documentID string) error {
	return c.CanDelete(ctx, tenantID, userID, ResourceTypeDocument, documentID)
}

// CanShareDocument is a convenience method for document share checks
func (c *Checker) CanShareDocument(ctx context.Context, tenantID uint32, userID string, documentID string) error {
	return c.CanShare(ctx, tenantID, userID, ResourceTypeDocument, documentID)
}

// CanDownloadDocument is a convenience method for document download checks
func (c *Checker) CanDownloadDocument(ctx context.Context, tenantID uint32, userID string, documentID string) error {
	return c.CanDownload(ctx, tenantID, userID, ResourceTypeDocument, documentID)
}

// GetEffectivePermissions returns all effective permissions for a user on a resource
func (c *Checker) GetEffectivePermissions(ctx context.Context, tenantID uint32, userID string, resourceType ResourceType, resourceID string) ([]Permission, Relation) {
	return c.engine.GetEffectivePermissions(ctx, CheckContext{
		TenantID:     tenantID,
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
	})
}

// ListAccessibleCategories lists all categories accessible by a user
func (c *Checker) ListAccessibleCategories(ctx context.Context, tenantID uint32, userID string) ([]string, error) {
	return c.engine.ListAccessibleResources(ctx, tenantID, userID, ResourceTypeCategory, PermissionRead)
}

// ListAccessibleDocuments lists all documents accessible by a user
func (c *Checker) ListAccessibleDocuments(ctx context.Context, tenantID uint32, userID string) ([]string, error) {
	return c.engine.ListAccessibleResources(ctx, tenantID, userID, ResourceTypeDocument, PermissionRead)
}
