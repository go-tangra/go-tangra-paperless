import { defineStore } from 'pinia';

import { paperlessApi } from '../api/client';
import type { Paging } from '../types';

// Document types (not in OpenAPI spec, using direct API calls)
type DocumentStatus =
  | 'DOCUMENT_STATUS_UNSPECIFIED'
  | 'DOCUMENT_STATUS_ACTIVE'
  | 'DOCUMENT_STATUS_ARCHIVED'
  | 'DOCUMENT_STATUS_DELETED';

type DocumentSource =
  | 'DOCUMENT_SOURCE_UNSPECIFIED'
  | 'DOCUMENT_SOURCE_UPLOAD'
  | 'DOCUMENT_SOURCE_SCAN'
  | 'DOCUMENT_SOURCE_EMAIL'
  | 'DOCUMENT_SOURCE_API';

interface Document {
  id?: string;
  tenantId?: number;
  name?: string;
  description?: string;
  categoryId?: string;
  fileName?: string;
  mimeType?: string;
  fileSize?: number;
  status?: DocumentStatus;
  source?: DocumentSource;
  tags?: Record<string, string>;
  createdAt?: string;
  updatedAt?: string;
}

interface ListDocumentsResponse {
  documents?: Document[];
  items?: Document[];
  total?: number;
}

interface GetDocumentResponse {
  document?: Document;
}

interface CreateDocumentResponse {
  document?: Document;
}

interface UpdateDocumentResponse {
  document?: Document;
}

interface DownloadDocumentResponse {
  content?: string;
  mimeType?: string;
  fileName?: string;
}

interface GetDocumentDownloadUrlResponse {
  url?: string;
  expiresAt?: string;
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

export const usePaperlessDocumentStore = defineStore('paperless-document', () => {
  /**
   * List documents in a category
   */
  async function listDocuments(
    paging?: Paging,
    formValues?: {
      categoryId?: string;
      nameFilter?: string;
      status?: DocumentStatus;
      source?: DocumentSource;
    } | null,
  ): Promise<ListDocumentsResponse> {
    return await paperlessApi.get<ListDocumentsResponse>(
      `/documents${buildQuery({
        categoryId: formValues?.categoryId,
        nameFilter: formValues?.nameFilter,
        status: formValues?.status,
        source: formValues?.source,
        page: paging?.page,
        pageSize: paging?.pageSize,
      })}`,
    );
  }

  /**
   * Get a document by ID
   */
  async function getDocument(id: string): Promise<GetDocumentResponse> {
    return await paperlessApi.get<GetDocumentResponse>(`/documents/${id}`);
  }

  /**
   * Create a new document
   */
  async function createDocument(request: {
    name?: string;
    description?: string;
    categoryId?: string;
    fileName?: string;
    fileContent?: string;
    mimeType?: string;
    tags?: Record<string, string>;
    source?: DocumentSource;
  }): Promise<CreateDocumentResponse> {
    return await paperlessApi.post<CreateDocumentResponse>('/documents', request);
  }

  /**
   * Update document metadata
   */
  async function updateDocument(
    id: string,
    request: {
      name?: string;
      description?: string;
      tags?: Record<string, string>;
    },
  ): Promise<UpdateDocumentResponse> {
    return await paperlessApi.put<UpdateDocumentResponse>(`/documents/${id}`, request);
  }

  /**
   * Delete a document
   */
  async function deleteDocument(id: string, permanent?: boolean): Promise<void> {
    return await paperlessApi.delete<void>(`/documents/${id}${buildQuery({ permanent })}`);
  }

  /**
   * Move document to a different category
   */
  async function moveDocument(id: string, newCategoryId?: string): Promise<void> {
    return await paperlessApi.post<void>(`/documents/${id}/move`, { newCategoryId });
  }

  /**
   * Download document content
   */
  async function downloadDocument(id: string): Promise<DownloadDocumentResponse> {
    return await paperlessApi.get<DownloadDocumentResponse>(`/documents/${id}/download`);
  }

  /**
   * Get presigned download URL
   */
  async function getDocumentDownloadUrl(
    id: string,
    expiresInSeconds?: number,
  ): Promise<GetDocumentDownloadUrlResponse> {
    return await paperlessApi.get<GetDocumentDownloadUrlResponse>(
      `/documents/${id}/download-url${buildQuery({ expiresInSeconds })}`,
    );
  }

  /**
   * Search documents across categories
   */
  async function searchDocuments(
    query: string,
    paging?: Paging,
    formValues?: {
      categoryId?: string;
      includeSubcategories?: boolean;
      status?: DocumentStatus;
      source?: DocumentSource;
    } | null,
  ): Promise<ListDocumentsResponse> {
    return await paperlessApi.get<ListDocumentsResponse>(
      `/documents/search${buildQuery({
        query,
        categoryId: formValues?.categoryId,
        includeSubcategories: formValues?.includeSubcategories,
        status: formValues?.status,
        source: formValues?.source,
        page: paging?.page,
        pageSize: paging?.pageSize,
      })}`,
    );
  }

  /**
   * Batch delete documents
   */
  async function batchDeleteDocuments(
    ids: string[],
    permanent?: boolean,
  ): Promise<void> {
    return await paperlessApi.post<void>('/documents/batch-delete', { ids, permanent });
  }

  /**
   * Upload a document with file content
   */
  async function uploadDocument(
    metadata: {
      name: string;
      description?: string;
      categoryId?: string;
      tags?: Record<string, string>;
    },
    file: File,
  ): Promise<CreateDocumentResponse> {
    // Read file as base64
    const fileContent = await fileToBase64(file);

    return await createDocument({
      name: metadata.name,
      description: metadata.description,
      categoryId: metadata.categoryId,
      fileName: file.name,
      fileContent,
      mimeType: file.type || 'application/octet-stream',
      tags: metadata.tags,
      source: 'DOCUMENT_SOURCE_UPLOAD',
    });
  }

  /**
   * Convert a File to base64 encoded string
   */
  function fileToBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => {
        // Remove the data URL prefix (e.g., "data:application/pdf;base64,")
        const result = reader.result as string;
        const base64 = result.split(',')[1];
        resolve(base64 ?? '');
      };
      reader.onerror = () => {
        reject(new Error(`Failed to read file: ${file.name}`));
      };
      reader.readAsDataURL(file);
    });
  }

  function $reset() {}

  return {
    $reset,
    listDocuments,
    getDocument,
    createDocument,
    updateDocument,
    deleteDocument,
    moveDocument,
    downloadDocument,
    getDocumentDownloadUrl,
    searchDocuments,
    batchDeleteDocuments,
    uploadDocument,
  };
});
