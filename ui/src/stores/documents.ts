import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ListQueryOptions } from '@go-tangra/ui'
import { api, upload, BASE } from '@/api/client'
import type { BatchResult, Document, DocumentInput, DocumentPage, ListParams, SearchHit } from '@/api/types'

export const PAGE_SIZE = 25

/** Sortable fields of GET /documents (server Spec store.DocumentList). */
export const DOCUMENT_SORTS = ['name', 'file_size', 'mime_type', 'status', 'processing_status', 'created_at'] as const
export const DOCUMENT_LIST: ListQueryOptions = { sortable: [...DOCUMENT_SORTS], defaultSort: { key: 'created_at', dir: 'desc' }, defaultSize: PAGE_SIZE }
const FIRST_PAGE: ListParams = { page: 1, page_size: PAGE_SIZE, sort: 'created_at', order: 'desc' }

export interface DocumentFilter {
  category_id?: string | undefined
  status?: string | undefined
  mime_type?: string | undefined
  source?: string | undefined
  processing_status?: string | undefined
  tag?: string | undefined
  created_by?: string | undefined
}

export interface UploadInput {
  file: File
  name?: string | undefined
  description?: string | undefined
  category_id?: string | undefined
  tags?: Record<string, string> | undefined
}

export const useDocuments = defineStore('paperless-documents', () => {
  const items = ref<Document[]>([])
  const total = ref(0)
  const params = ref<ListParams>({ ...FIRST_PAGE })
  const filter = ref<DocumentFilter>({})
  const loading = ref(false)
  const error = ref('')
  let seq = 0

  /**
   * Loads one server page with the filter (blank filter values are not sent).
   * Resolves with the page, or null when it failed or a newer request
   * superseded it (its rows are then ignored).
   */
  async function list(f: DocumentFilter = filter.value, q: ListParams = params.value): Promise<DocumentPage | null> {
    const mine = ++seq
    loading.value = true
    error.value = ''
    filter.value = { ...f }
    params.value = { ...q }
    try {
      const res = await api<DocumentPage>('GET', 'documents', undefined, { query: { ...f, ...q } })
      if (mine !== seq) return null
      items.value = res.items ?? []
      total.value = res.total ?? 0
      return res
    } catch (e) {
      if (mine === seq) error.value = (e as Error).message
      return null
    } finally {
      if (mine === seq) loading.value = false
    }
  }

  /** Reloads the current page with the current filter and order. */
  const reload = () => list()

  async function get(id: string, includeContent = false): Promise<Document> {
    return api<Document>('GET', 'documents/' + id, undefined, { query: { include_content: includeContent } })
  }

  // upload posts a multipart/form-data body; the browser sets the boundary.
  async function uploadDocument(input: UploadInput): Promise<Document> {
    const fields: Record<string, string> = {}
    if (input.name) fields.name = input.name
    if (input.description) fields.description = input.description
    if (input.category_id) fields.category_id = input.category_id
    if (input.tags && Object.keys(input.tags).length) fields.tags = JSON.stringify(input.tags)
    // The caller reloads the current page: a new document takes its place in
    // the server's order rather than being prepended to this page.
    return upload<Document>('documents', input.file, fields)
  }

  async function update(id: string, input: DocumentInput): Promise<Document> {
    const d = await api<Document>('PUT', 'documents/' + id, input)
    items.value = items.value.map((x) => (x.id === id ? d : x))
    return d
  }

  async function remove(id: string, hard = false): Promise<void> {
    await api('POST', 'documents/' + id + '/remove', undefined, { query: { hard } })
    items.value = items.value.filter((x) => x.id !== id)
  }

  async function move(id: string, categoryId: string): Promise<Document> {
    const d = await api<Document>('POST', 'documents/' + id + '/move', { category_id: categoryId })
    items.value = items.value.map((x) => (x.id === id ? d : x))
    return d
  }

  // downloadUrl asks the service for a (short-lived) object-store URL.
  async function downloadUrl(id: string): Promise<string> {
    const res = await api<{ url: string }>('GET', 'documents/' + id + '/download-url')
    return res.url
  }

  // directDownload streams the bytes through the gateway.
  function directDownload(id: string): string {
    return BASE + '/documents/' + id + '/download'
  }

  async function search(query: string, limit = 20): Promise<SearchHit[]> {
    const res = await api<{ items: SearchHit[] }>('POST', 'documents/search', { query, limit })
    return res.items ?? []
  }

  async function batchDelete(ids: string[], hard = false): Promise<BatchResult[]> {
    const res = await api<{ results: BatchResult[] }>('POST', 'documents/batch-delete', { ids, hard })
    const ok = new Set(res.results.filter((r) => r.ok).map((r) => r.id))
    items.value = items.value.filter((x) => !ok.has(x.id))
    return res.results ?? []
  }

  // patchProcessing updates one document's processing_status in place (live events).
  function patchProcessing(id: string, status: string): void {
    const i = items.value.findIndex((x) => x.id === id)
    if (i >= 0) {
      const cur = items.value[i]
      if (cur) items.value[i] = { ...cur, processing_status: status as Document['processing_status'] }
    }
  }

  return { items, total, params, filter, loading, error, list, reload, get, upload: uploadDocument, update, remove, move, downloadUrl, directDownload, search, batchDelete, patchProcessing }
})
