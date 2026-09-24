import { z } from 'zod'
import { nonEmpty, optionalString } from '@go-tangra/ui/forms'

export const categorySchema = z.object({
  name: nonEmpty(120).refine((s) => !s.includes('/'), 'Names cannot contain "/".'),
  parent_id: optionalString(64),
})
export type CategoryInput = z.output<typeof categorySchema>

export const moveCategorySchema = z.object({ parent_id: optionalString(64) })
