import { z } from 'zod'
import { nonEmpty, optionalString, tagMap } from '@freya/ui/forms'

export const DOCUMENT_STATUSES = ['active', 'archived', 'deleted'] as const
export const PROCESSING_STATUSES = ['pending', 'processing', 'completed', 'failed', 'retrying'] as const

/** PUT /documents/{id} payload. */
export const documentSchema = z.object({
  name: nonEmpty(255),
  description: optionalString(2000),
  category_id: optionalString(64),
  tags: tagMap(32),
})
export type DocumentInput = z.output<typeof documentSchema>

export const documentFilterSchema = z.object({
  status: z.enum(DOCUMENT_STATUSES).optional(),
  processing_status: z.enum(PROCESSING_STATUSES).optional(),
})
