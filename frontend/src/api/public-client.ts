/**
 * Public (unauthenticated) client for document signing sessions.
 *
 * Uses the public nginx route: /public/v1/signing/sessions/{token}
 */

const PUBLIC_BASE_URL = '/public/v1/signing';

export interface SigningField {
  fieldId: string;
  name: string;
  type: string;
  required: boolean;
  pageNumber: number;
  xPercent: number;
  yPercent: number;
  widthPercent: number;
  heightPercent: number;
  prefilledValue: string;
  recipientIndex: number;
}

export interface SigningSessionResponse {
  requestName: string;
  documentUrl: string;
  recipientName: string;
  recipientEmail: string;
  fields: SigningField[];
  message: string;
  expiresAt: string;
}

export interface SubmitSigningRequest {
  fieldValues: Array<{
    fieldId: string;
    value: string;
    pageNumber?: number;
    xPercent?: number;
    yPercent?: number;
    widthPercent?: number;
    heightPercent?: number;
  }>;
  signatureImage?: string;
  signaturePageNumber?: number;
  signatureXPercent?: number;
  signatureYPercent?: number;
  signatureWidthPercent?: number;
  signatureHeightPercent?: number;
}

export interface SubmitSigningResponse {
  message: string;
  completedAt?: string;
}

export async function getSigningSession(
  token: string,
): Promise<SigningSessionResponse> {
  const response = await fetch(`${PUBLIC_BASE_URL}/sessions/${token}`);

  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(
      data.error || data.message || `Failed to load signing session (${response.status})`,
    );
  }

  return response.json();
}

export async function submitSigning(
  token: string,
  data: SubmitSigningRequest,
): Promise<SubmitSigningResponse> {
  const response = await fetch(`${PUBLIC_BASE_URL}/sessions/${token}/submit`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    const errData = await response.json().catch(() => ({}));
    throw new Error(
      errData.error || errData.message || `Failed to submit signing (${response.status})`,
    );
  }

  return response.json();
}
