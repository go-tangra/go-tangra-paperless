/**
 * Paperless Module Service Functions
 *
 * Typed service methods for the Paperless API using dynamic module routing.
 * Base URL: /admin/v1/modules/paperless/v1
 */

import type { components, operations } from './types';
import { paperlessApi, type RequestOptions } from './client';

// Entity type aliases
export type Category = components['schemas']['Category'];
export type CategoryTreeNode = components['schemas']['CategoryTreeNode'];
export type Document = components['schemas']['Document'];
export type PermissionTuple = components['schemas']['PermissionTuple'];

// Enum types derived from entity fields
export type DocumentStatus = NonNullable<Document['status']>;
export type DocumentSource = NonNullable<Document['source']>;
export type ResourceType = NonNullable<PermissionTuple['resourceType']>;
export type Relation = NonNullable<PermissionTuple['relation']>;
export type SubjectType = NonNullable<PermissionTuple['subjectType']>;

// List Response types
export type ListCategoriesResponse = components['schemas']['ListCategoriesResponse'];
export type ListDocumentsResponse = components['schemas']['ListDocumentsResponse'];
export type ListPermissionsResponse = components['schemas']['ListPermissionsResponse'];
export type ListAccessibleResourcesResponse = components['schemas']['ListAccessibleResourcesResponse'];

// Get Response types
export type GetCategoryResponse = components['schemas']['GetCategoryResponse'];
export type GetCategoryTreeResponse = components['schemas']['GetCategoryTreeResponse'];
export type GetDocumentResponse = components['schemas']['GetDocumentResponse'];
export type GetDocumentDownloadUrlResponse = components['schemas']['GetDocumentDownloadUrlResponse'];
export type GetEffectivePermissionsResponse = components['schemas']['GetEffectivePermissionsResponse'];
export type DownloadDocumentResponse = components['schemas']['DownloadDocumentResponse'];

// Create Response types
export type CreateCategoryResponse = components['schemas']['CreateCategoryResponse'];
export type CreateDocumentResponse = components['schemas']['CreateDocumentResponse'];

// Update Response types
export type UpdateCategoryResponse = components['schemas']['UpdateCategoryResponse'];
export type UpdateDocumentResponse = components['schemas']['UpdateDocumentResponse'];

// Move Response types
export type MoveCategoryResponse = components['schemas']['MoveCategoryResponse'];
export type MoveDocumentResponse = components['schemas']['MoveDocumentResponse'];

// Other Response types
export type BatchDeleteDocumentsResponse = components['schemas']['BatchDeleteDocumentsResponse'];
export type SearchDocumentsResponse = components['schemas']['SearchDocumentsResponse'];
export type CheckAccessResponse = components['schemas']['CheckAccessResponse'];
export type GrantAccessResponse = components['schemas']['GrantAccessResponse'];

// Request types
export type CreateCategoryRequest = components['schemas']['CreateCategoryRequest'];
export type UpdateCategoryRequest = components['schemas']['UpdateCategoryRequest'];
export type MoveCategoryRequest = components['schemas']['MoveCategoryRequest'];
export type CreateDocumentRequest = components['schemas']['CreateDocumentRequest'];
export type UpdateDocumentRequest = components['schemas']['UpdateDocumentRequest'];
export type MoveDocumentRequest = components['schemas']['MoveDocumentRequest'];
export type BatchDeleteDocumentsRequest = components['schemas']['BatchDeleteDocumentsRequest'];
export type GrantAccessRequest = components['schemas']['GrantAccessRequest'];
export type CheckAccessRequest = components['schemas']['CheckAccessRequest'];

// Helper to build query string
function buildQuery(params: Record<string, unknown>): string {
  const searchParams = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== '') {
      if (Array.isArray(value)) {
        value.forEach(v => searchParams.append(key, String(v)));
      } else {
        searchParams.append(key, String(value));
      }
    }
  }
  const query = searchParams.toString();
  return query ? `?${query}` : '';
}

// ==================== Category Service ====================

export const CategoryService = {
  list: async (
    params?: operations['PaperlessCategoryService_ListCategories']['parameters']['query'],
    options?: RequestOptions
  ): Promise<ListCategoriesResponse> => {
    return paperlessApi.get<ListCategoriesResponse>(`/categories${buildQuery(params || {})}`, options);
  },

  get: async (
    id: string,
    includeCounts?: boolean,
    options?: RequestOptions
  ): Promise<GetCategoryResponse> => {
    return paperlessApi.get<GetCategoryResponse>(`/categories/${id}${buildQuery({ includeCounts })}`, options);
  },

  create: async (
    data: CreateCategoryRequest,
    options?: RequestOptions
  ): Promise<CreateCategoryResponse> => {
    return paperlessApi.post<CreateCategoryResponse>('/categories', data, options);
  },

  update: async (
    id: string,
    data: Omit<UpdateCategoryRequest, 'id'>,
    options?: RequestOptions
  ): Promise<UpdateCategoryResponse> => {
    return paperlessApi.put<UpdateCategoryResponse>(`/categories/${id}`, { ...data, id }, options);
  },

  delete: async (id: string, force?: boolean, options?: RequestOptions): Promise<void> => {
    return paperlessApi.delete<void>(`/categories/${id}${buildQuery({ force })}`, options);
  },

  getTree: async (
    params?: operations['PaperlessCategoryService_GetCategoryTree']['parameters']['query'],
    options?: RequestOptions
  ): Promise<GetCategoryTreeResponse> => {
    return paperlessApi.get<GetCategoryTreeResponse>(`/categories/tree${buildQuery(params || {})}`, options);
  },

  move: async (
    id: string,
    newParentId?: string,
    options?: RequestOptions
  ): Promise<MoveCategoryResponse> => {
    return paperlessApi.post<MoveCategoryResponse>(`/categories/${id}/move`, { id, newParentId }, options);
  },
};

// ==================== Document Service ====================

export const DocumentService = {
  list: async (
    params?: operations['PaperlessDocumentService_ListDocuments']['parameters']['query'],
    options?: RequestOptions
  ): Promise<ListDocumentsResponse> => {
    return paperlessApi.get<ListDocumentsResponse>(`/documents${buildQuery(params || {})}`, options);
  },

  get: async (id: string, options?: RequestOptions): Promise<GetDocumentResponse> => {
    return paperlessApi.get<GetDocumentResponse>(`/documents/${id}`, options);
  },

  create: async (
    data: CreateDocumentRequest,
    options?: RequestOptions
  ): Promise<CreateDocumentResponse> => {
    return paperlessApi.post<CreateDocumentResponse>('/documents', data, options);
  },

  update: async (
    id: string,
    data: Omit<UpdateDocumentRequest, 'id'>,
    options?: RequestOptions
  ): Promise<UpdateDocumentResponse> => {
    return paperlessApi.put<UpdateDocumentResponse>(`/documents/${id}`, { ...data, id }, options);
  },

  delete: async (id: string, permanent?: boolean, options?: RequestOptions): Promise<void> => {
    return paperlessApi.delete<void>(`/documents/${id}${buildQuery({ permanent })}`, options);
  },

  batchDelete: async (
    data: BatchDeleteDocumentsRequest,
    options?: RequestOptions
  ): Promise<BatchDeleteDocumentsResponse> => {
    return paperlessApi.post<BatchDeleteDocumentsResponse>('/documents/batch-delete', data, options);
  },

  search: async (
    params?: operations['PaperlessDocumentService_SearchDocuments']['parameters']['query'],
    options?: RequestOptions
  ): Promise<SearchDocumentsResponse> => {
    return paperlessApi.get<SearchDocumentsResponse>(`/documents/search${buildQuery(params || {})}`, options);
  },

  download: async (id: string, options?: RequestOptions): Promise<DownloadDocumentResponse> => {
    return paperlessApi.get<DownloadDocumentResponse>(`/documents/${id}/download`, options);
  },

  getDownloadUrl: async (
    id: string,
    expiresIn?: number,
    options?: RequestOptions
  ): Promise<GetDocumentDownloadUrlResponse> => {
    return paperlessApi.get<GetDocumentDownloadUrlResponse>(
      `/documents/${id}/download-url${buildQuery({ expiresIn })}`,
      options
    );
  },

  move: async (
    id: string,
    newCategoryId?: string,
    options?: RequestOptions
  ): Promise<MoveDocumentResponse> => {
    return paperlessApi.post<MoveDocumentResponse>(`/documents/${id}/move`, { id, newCategoryId }, options);
  },
};

// ==================== Permission Service ====================

// ==================== Permission Service ====================

export const PermissionService = {
  list: async (
    params?: operations['PaperlessPermissionService_ListPermissions']['parameters']['query'],
    options?: RequestOptions
  ): Promise<ListPermissionsResponse> => {
    return paperlessApi.get<ListPermissionsResponse>(`/permissions${buildQuery(params || {})}`, options);
  },

  grant: async (
    data: GrantAccessRequest,
    options?: RequestOptions
  ): Promise<GrantAccessResponse> => {
    return paperlessApi.post<GrantAccessResponse>('/permissions', data, options);
  },

  revoke: async (
    params: operations['PaperlessPermissionService_RevokeAccess']['parameters']['query'],
    options?: RequestOptions
  ): Promise<void> => {
    return paperlessApi.delete<void>(`/permissions${buildQuery(params || {})}`, options);
  },

  check: async (
    data: CheckAccessRequest,
    options?: RequestOptions
  ): Promise<CheckAccessResponse> => {
    return paperlessApi.post<CheckAccessResponse>('/permissions/check', data, options);
  },

  listAccessible: async (
    params?: operations['PaperlessPermissionService_ListAccessibleResources']['parameters']['query'],
    options?: RequestOptions
  ): Promise<ListAccessibleResourcesResponse> => {
    return paperlessApi.get<ListAccessibleResourcesResponse>(
      `/permissions/accessible${buildQuery(params || {})}`,
      options
    );
  },

  getEffective: async (
    params?: operations['PaperlessPermissionService_GetEffectivePermissions']['parameters']['query'],
    options?: RequestOptions
  ): Promise<GetEffectivePermissionsResponse> => {
    return paperlessApi.get<GetEffectivePermissionsResponse>(
      `/permissions/effective${buildQuery(params || {})}`,
      options
    );
  },
};
