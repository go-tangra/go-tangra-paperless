import { defineStore } from 'pinia';

import { paperlessApi } from '../api/client';
import type { Paging } from '../types';

// Permission types (not in OpenAPI spec, using direct API calls)
type SubjectType = 'SUBJECT_TYPE_UNSPECIFIED' | 'SUBJECT_TYPE_USER' | 'SUBJECT_TYPE_GROUP';
type ResourceType =
  | 'RESOURCE_TYPE_UNSPECIFIED'
  | 'RESOURCE_TYPE_CATEGORY'
  | 'RESOURCE_TYPE_DOCUMENT';
type Relation =
  | 'RELATION_UNSPECIFIED'
  | 'RELATION_OWNER'
  | 'RELATION_EDITOR'
  | 'RELATION_VIEWER';

interface Permission {
  id?: string;
  resourceType?: ResourceType;
  resourceId?: string;
  subjectType?: SubjectType;
  subjectId?: string;
  relation?: Relation;
  createdAt?: string;
}

interface ListPermissionsResponse {
  permissions?: Permission[];
  total?: number;
}

interface GrantAccessResponse {
  permission?: Permission;
}

interface CheckAccessResponse {
  hasAccess?: boolean;
}

interface AccessibleResource {
  resourceId?: string;
  relation?: Relation;
}

interface ListAccessibleResourcesResponse {
  resources?: AccessibleResource[];
  total?: number;
}

interface GetEffectivePermissionsResponse {
  permissions?: Relation[];
}

// Helper to build query string
function buildQuery(params: Record<string, unknown>): string {
  const searchParams = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== '') {
      searchParams.append(key, String(value));
    }
  }
  const query = searchParams.toString();
  return query ? `?${query}` : '';
}

export const usePaperlessPermissionStore = defineStore(
  'paperless-permission',
  () => {
    /**
     * Grant access to a resource
     */
    async function grantAccess(request: {
      resourceType: ResourceType;
      resourceId: string;
      subjectType: SubjectType;
      subjectId: string;
      relation: Relation;
    }): Promise<GrantAccessResponse> {
      return await paperlessApi.post<GrantAccessResponse>('/permissions', request);
    }

    /**
     * Revoke access from a resource
     */
    async function revokeAccess(
      resourceType: ResourceType,
      resourceId: string,
      subjectType: SubjectType,
      subjectId: string,
      relation: Relation,
    ): Promise<void> {
      return await paperlessApi.delete<void>(
        `/permissions${buildQuery({
          resourceType,
          resourceId,
          subjectType,
          subjectId,
          relation,
        })}`,
      );
    }

    /**
     * List permissions for a resource
     */
    async function listPermissions(
      resourceType: ResourceType,
      resourceId: string,
      paging?: Paging,
    ): Promise<ListPermissionsResponse> {
      return await paperlessApi.get<ListPermissionsResponse>(
        `/permissions${buildQuery({
          resourceType,
          resourceId,
          page: paging?.page,
          pageSize: paging?.pageSize,
        })}`,
      );
    }

    /**
     * Check if subject has access to a resource
     */
    async function checkAccess(
      resourceType: ResourceType,
      resourceId: string,
      subjectType: SubjectType,
      subjectId: string,
      relation: Relation,
    ): Promise<CheckAccessResponse> {
      return await paperlessApi.post<CheckAccessResponse>('/permissions/check', {
        resourceType,
        resourceId,
        subjectType,
        subjectId,
        relation,
      });
    }

    /**
     * List resources accessible by a subject
     */
    async function listAccessibleResources(
      subjectType: SubjectType,
      subjectId: string,
      resourceType: ResourceType,
      relation?: Relation,
      paging?: Paging,
    ): Promise<ListAccessibleResourcesResponse> {
      return await paperlessApi.get<ListAccessibleResourcesResponse>(
        `/permissions/accessible${buildQuery({
          subjectType,
          subjectId,
          resourceType,
          relation,
          page: paging?.page,
          pageSize: paging?.pageSize,
        })}`,
      );
    }

    /**
     * Get effective permissions for a subject on a resource
     */
    async function getEffectivePermissions(
      resourceType: ResourceType,
      resourceId: string,
      subjectType: SubjectType,
      subjectId: string,
    ): Promise<GetEffectivePermissionsResponse> {
      return await paperlessApi.get<GetEffectivePermissionsResponse>(
        `/permissions/effective${buildQuery({
          resourceType,
          resourceId,
          subjectType,
          subjectId,
        })}`,
      );
    }

    function $reset() {}

    return {
      $reset,
      grantAccess,
      revokeAccess,
      listPermissions,
      checkAccess,
      listAccessibleResources,
      getEffectivePermissions,
    };
  },
);
