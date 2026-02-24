import { defineStore } from 'pinia';

import { paperlessApi } from '../api/client';
import type { Paging } from '../types';

// Signing Template types
export type SigningFieldType =
  | 'SIGNING_FIELD_TYPE_UNSPECIFIED'
  | 'SIGNING_FIELD_TYPE_TEXT'
  | 'SIGNING_FIELD_TYPE_SIGNATURE'
  | 'SIGNING_FIELD_TYPE_DATE'
  | 'SIGNING_FIELD_TYPE_INITIALS'
  | 'SIGNING_FIELD_TYPE_CHECKBOX'
  | 'SIGNING_FIELD_TYPE_EMAIL';

export interface SigningTemplateField {
  id?: string;
  name?: string;
  type?: SigningFieldType;
  required?: boolean;
  pageNumber?: number;
  xPercent?: number;
  yPercent?: number;
  widthPercent?: number;
  heightPercent?: number;
  prefillStage?: number;
  recipientIndex?: number;
}

export interface SigningTemplate {
  id?: string;
  name?: string;
  description?: string;
  fileKey?: string;
  fileName?: string;
  fileSize?: number;
  fields?: SigningTemplateField[];
  createTime?: string;
  updateTime?: string;
}

interface ListSigningTemplatesResponse {
  templates?: SigningTemplate[];
  total?: number;
}

interface GetSigningTemplateResponse {
  template?: SigningTemplate;
}

interface CreateSigningTemplateResponse {
  template?: SigningTemplate;
}

interface UpdateSigningTemplateResponse {
  template?: SigningTemplate;
}

interface UpdateTemplateFieldsResponse {
  template?: SigningTemplate;
}

interface GetTemplatePdfUrlResponse {
  url?: string;
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

export const usePaperlessSigningTemplateStore = defineStore('paperless-signing-template', () => {
  async function listSigningTemplates(
    paging?: Paging,
    formValues?: { nameFilter?: string } | null,
  ): Promise<ListSigningTemplatesResponse> {
    return await paperlessApi.get<ListSigningTemplatesResponse>(
      `/signing/templates${buildQuery({
        nameFilter: formValues?.nameFilter,
        page: paging?.page,
        pageSize: paging?.pageSize,
      })}`,
    );
  }

  async function getSigningTemplate(id: string): Promise<GetSigningTemplateResponse> {
    return await paperlessApi.get<GetSigningTemplateResponse>(`/signing/templates/${id}`);
  }

  async function createSigningTemplate(
    metadata: { name: string; description?: string },
    file: File,
  ): Promise<CreateSigningTemplateResponse> {
    const fileContent = await fileToBase64(file);
    return await paperlessApi.post<CreateSigningTemplateResponse>('/signing/templates', {
      name: metadata.name,
      description: metadata.description,
      fileName: file.name,
      fileContent,
    });
  }

  async function updateSigningTemplate(
    id: string,
    data: { name?: string; description?: string },
  ): Promise<UpdateSigningTemplateResponse> {
    return await paperlessApi.put<UpdateSigningTemplateResponse>(`/signing/templates/${id}`, data);
  }

  async function deleteSigningTemplate(id: string): Promise<void> {
    return await paperlessApi.delete<void>(`/signing/templates/${id}`);
  }

  async function updateTemplateFields(
    id: string,
    fields: SigningTemplateField[],
  ): Promise<UpdateTemplateFieldsResponse> {
    return await paperlessApi.put<UpdateTemplateFieldsResponse>(`/signing/templates/${id}/fields`, {
      fields,
    });
  }

  async function getTemplatePdfUrl(id: string): Promise<GetTemplatePdfUrlResponse> {
    return await paperlessApi.get<GetTemplatePdfUrlResponse>(`/signing/templates/${id}/pdf-url`);
  }

  function fileToBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => {
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
    listSigningTemplates,
    getSigningTemplate,
    createSigningTemplate,
    updateSigningTemplate,
    deleteSigningTemplate,
    updateTemplateFields,
    getTemplatePdfUrl,
  };
});
