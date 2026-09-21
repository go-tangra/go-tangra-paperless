import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, upload, BASE } from '@/api/client'
import type { BatchResult, Document, DocumentInput, SearchHit } from '@/api/types'

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
  const loading = ref(false)
  const error = ref('')

  async function list(filter: DocumentFilter = {}): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const res = await api<{ items: Document[] }>('GET', 'documents', undefined, { query: { ...filter } })
      items.value = res.items ?? []
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  async function get(id: string, includeContent = false): Promise<Document> {
    return api<Document>('GET', 'documents/' + id, undefined, { query: { include_content: includeContent } })
  }

  // upload posts a multipart/form-data body; the browser sets the boundary.
  async function uploadDocument(input: UploadInput): Promise<Document> {
    const form = new FormData()
    form.append('file', input.file)
    if (input.name) form.append('name', input.name)
    if (input.description) form.append('description', input.description)
    if (input.category_id) form.append('category_id', input.category_id)
    if (input.tags && Object.keys(input.tags).length) form.append('tags', JSON.stringify(input.tags))
    const d = await upload<Document>('documents', form)
    items.value = [d, ...items.value]
    return d
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

  return { items, loading, error, list, get, upload: uploadDocument, update, remove, move, downloadUrl, directDownload, search, batchDelete, patchProcessing }
})
