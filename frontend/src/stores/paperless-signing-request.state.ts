import { defineStore } from 'pinia';

import { paperlessApi } from '../api/client';
import type { Paging } from '../types';

// Signing Request types
export type SigningRequestStatus =
  | 'SIGNING_REQUEST_STATUS_UNSPECIFIED'
  | 'SIGNING_REQUEST_STATUS_DRAFT'
  | 'SIGNING_REQUEST_STATUS_PENDING'
  | 'SIGNING_REQUEST_STATUS_COMPLETED'
  | 'SIGNING_REQUEST_STATUS_CANCELLED'
  | 'SIGNING_REQUEST_STATUS_EXPIRED';

export type SigningRecipientStatus =
  | 'SIGNING_RECIPIENT_STATUS_UNSPECIFIED'
  | 'SIGNING_RECIPIENT_STATUS_WAITING'
  | 'SIGNING_RECIPIENT_STATUS_PENDING'
  | 'SIGNING_RECIPIENT_STATUS_COMPLETED'
  | 'SIGNING_RECIPIENT_STATUS_DECLINED';

export interface SigningRecipient {
  id?: string;
  email?: string;
  name?: string;
  signingOrder?: number;
  status?: SigningRecipientStatus;
  signedAt?: string;
}

export interface SigningFieldValue {
  fieldId?: string;
  fieldName?: string;
  value?: string;
  filledBy?: string;
}

export interface SigningRequest {
  id?: string;
  templateId?: string;
  templateName?: string;
  name?: string;
  status?: SigningRequestStatus;
  originalFileKey?: string;
  signedFileKey?: string;
  recipients?: SigningRecipient[];
  fieldValues?: SigningFieldValue[];
  message?: string;
  expiresAt?: string;
  createTime?: string;
  updateTime?: string;
}

interface ListSigningRequestsResponse {
  requests?: SigningRequest[];
  total?: number;
}

interface GetSigningRequestResponse {
  request?: SigningRequest;
}

interface CreateSigningRequestResponse {
  request?: SigningRequest;
}

interface DownloadSignedDocumentResponse {
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

export const usePaperlessSigningRequestStore = defineStore('paperless-signing-request', () => {
  async function listSigningRequests(
    paging?: Paging,
    formValues?: { status?: SigningRequestStatus } | null,
  ): Promise<ListSigningRequestsResponse> {
    return await paperlessApi.get<ListSigningRequestsResponse>(
      `/signing/requests${buildQuery({
        status: formValues?.status,
        page: paging?.page,
        pageSize: paging?.pageSize,
      })}`,
    );
  }

  async function getSigningRequest(id: string): Promise<GetSigningRequestResponse> {
    return await paperlessApi.get<GetSigningRequestResponse>(`/signing/requests/${id}`);
  }

  async function createSigningRequest(data: {
    templateId: string;
    name: string;
    recipients: Array<{ email: string; name: string; signingOrder: number }>;
    fieldValues?: Array<{ fieldId: string; value: string }>;
    message?: string;
    expiresAt?: string;
  }): Promise<CreateSigningRequestResponse> {
    return await paperlessApi.post<CreateSigningRequestResponse>('/signing/requests', data);
  }

  async function cancelSigningRequest(id: string): Promise<void> {
    return await paperlessApi.post<void>(`/signing/requests/${id}/cancel`, {});
  }

  async function resendSigningEmail(id: string, recipientId: string): Promise<void> {
    return await paperlessApi.post<void>(`/signing/requests/${id}/resend`, { recipientId });
  }

  async function downloadSignedDocument(id: string): Promise<DownloadSignedDocumentResponse> {
    return await paperlessApi.get<DownloadSignedDocumentResponse>(`/signing/requests/${id}/download`);
  }

  function $reset() {}

  return {
    $reset,
    listSigningRequests,
    getSigningRequest,
    createSigningRequest,
    cancelSigningRequest,
    resendSigningEmail,
    downloadSignedDocument,
  };
});
