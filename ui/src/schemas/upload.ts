import { z } from 'zod'
import { optionalString, tagMap } from '@freya/ui/forms'

export const MAX_UPLOAD_BYTES = 100 * 1024 * 1024

/** Multipart POST /documents: one file plus optional metadata. */
export const uploadSchema = z.object({
  file: z.instanceof(File, { message: 'Choose a file to upload.' }).refine((f) => f.size > 0, 'The file is empty.').refine((f) => f.size <= MAX_UPLOAD_BYTES, 'The file is larger than 100 MiB.'),
  name: optionalString(255),
  description: optionalString(2000),
  category_id: optionalString(64),
  tags: tagMap(32),
})
export type UploadInput = z.output<typeof uploadSchema>
