import { defineStore } from 'pinia';

import { paperlessApi } from '../api/client';
import type { Paging } from '../types';

// Category types (not in OpenAPI spec, using direct API calls)
interface Category {
  id?: string;
  tenantId?: number;
  name?: string;
  description?: string;
  parentId?: string;
  path?: string;
  documentCount?: number;
  childCount?: number;
  createdAt?: string;
  updatedAt?: string;
}

interface ListCategoriesResponse {
  categories?: Category[];
  items?: Category[];
  total?: number;
}

interface GetCategoryResponse {
  category?: Category;
}

interface CreateCategoryResponse {
  category?: Category;
}

interface UpdateCategoryResponse {
  category?: Category;
}

interface CategoryTreeNode {
  category?: Category;
  children?: CategoryTreeNode[];
}

interface GetCategoryTreeResponse {
  roots?: CategoryTreeNode[];
  nodes?: CategoryTreeNode[];
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

export const usePaperlessCategoryStore = defineStore('paperless-category', () => {
  /**
   * List categories
   */
  async function listCategories(
    paging?: Paging,
    formValues?: {
      parentId?: string;
      nameFilter?: string;
    } | null,
  ): Promise<ListCategoriesResponse> {
    return await paperlessApi.get<ListCategoriesResponse>(
      `/categories${buildQuery({
        parentId: formValues?.parentId,
        nameFilter: formValues?.nameFilter,
        page: paging?.page,
        pageSize: paging?.pageSize,
      })}`,
    );
  }

  /**
   * Get a category by ID
   */
  async function getCategory(
    id: string,
    includeDocumentCount?: boolean,
  ): Promise<GetCategoryResponse> {
    return await paperlessApi.get<GetCategoryResponse>(
      `/categories/${id}${buildQuery({ includeDocumentCount })}`,
    );
  }

  /**
   * Create a new category
   */
  async function createCategory(request: {
    name?: string;
    description?: string;
    parentId?: string;
  }): Promise<CreateCategoryResponse> {
    return await paperlessApi.post<CreateCategoryResponse>('/categories', request);
  }

  /**
   * Update category metadata
   */
  async function updateCategory(
    id: string,
    request: {
      name?: string;
      description?: string;
    },
  ): Promise<UpdateCategoryResponse> {
    return await paperlessApi.put<UpdateCategoryResponse>(`/categories/${id}`, request);
  }

  /**
   * Delete a category
   */
  async function deleteCategory(id: string, force?: boolean): Promise<void> {
    return await paperlessApi.delete<void>(`/categories/${id}${buildQuery({ force })}`);
  }

  /**
   * Move a category to a new parent
   */
  async function moveCategory(id: string, newParentId?: string): Promise<void> {
    return await paperlessApi.post<void>(`/categories/${id}/move`, { newParentId });
  }

  /**
   * Get the category tree structure
   */
  async function getCategoryTree(
    rootId?: string,
    maxDepth?: number,
    includeDocumentCount?: boolean,
  ): Promise<GetCategoryTreeResponse> {
    return await paperlessApi.get<GetCategoryTreeResponse>(
      `/categories/tree${buildQuery({ rootId, maxDepth, includeDocumentCount })}`,
    );
  }

  function $reset() {}

  return {
    $reset,
    listCategories,
    getCategory,
    createCategory,
    updateCategory,
    deleteCategory,
    moveCategory,
    getCategoryTree,
  };
});
