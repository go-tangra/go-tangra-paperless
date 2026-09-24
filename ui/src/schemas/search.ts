import { z } from 'zod'

export const searchSchema = z.object({
  query: z.string().trim().min(2, 'Type at least two characters.').max(200),
})
export type SearchInput = z.output<typeof searchSchema>
