import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import Documents from '@/views/documents/index.vue'
import Categories from '@/views/categories/index.vue'
import Search from '@/views/search/index.vue'
import Dashboard from '@/views/dashboard/index.vue'
import { uploadSchema, documentSchema, categorySchema, searchSchema } from '@/schemas'

function fetchMock(handler: (url: string, init: RequestInit) => unknown) {
  const calls: { url: string; init: RequestInit }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init: RequestInit = {}) => {
    calls.push({ url, init })
    return new Response(JSON.stringify(handler(url, init)), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }))
  return calls
}
class FakeSource { onopen = null; onerror = null; addEventListener() {} close() {} }
const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div/>' } }] })
const global = { plugins: [router] }
const doc = { id: 'd1', tenant_id: 't', name: 'Invoice', file_name: 'inv.pdf', file_size: 2048, mime_type: 'application/pdf', checksum: 'abc', status: 'active', source: 'upload', processing_status: 'completed', created_by: 'u1', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', tags: { year: '2026' } }
const cat = { id: 'c1', parent_id: null, name: 'Finance', path: '/Finance', depth: 0, sort_order: 0, document_count: 1, subcategory_count: 0 }

describe('paperless schemas', () => {
  it('upload needs a non-empty file; edit needs a name; category names have no slash; search min length', () => {
    expect(uploadSchema.safeParse({}).success).toBe(false)
    expect(uploadSchema.safeParse({ file: new File([], 'e.txt') }).success).toBe(false)
    expect(uploadSchema.safeParse({ file: new File(['x'], 'a.txt'), name: ' A ', tags: { k: 'v' } }).data).toMatchObject({ name: 'A', tags: { k: 'v' } })
    expect(documentSchema.safeParse({ name: '' }).success).toBe(false)
    expect(documentSchema.safeParse({ name: 'x', category_id: '', tags: null }).data).toEqual({ name: 'x', tags: {} })
    expect(categorySchema.safeParse({ name: 'a/b' }).success).toBe(false)
    expect(searchSchema.safeParse({ query: 'a' }).success).toBe(false)
  })
})

describe('paperless views on the kit', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    document.cookie = '__Host-csrf=tok; Secure; Path=/'
    vi.stubGlobal('EventSource', FakeSource)
    ;(globalThis as unknown as { __vw: number }).__vw = 1280
  })

  it('documents: table, upload dialog blocks without a file then posts multipart, drawer edits', async () => {
    const calls = fetchMock((url, init) => (init.method === 'POST' && url.endsWith('/documents') ? { ...doc, id: 'd2', name: 'New' } : init.method === 'PUT' ? { ...doc, name: 'Renamed' } : url.includes('/categories') ? { items: [cat] } : { items: [doc] }))
    const w = mount(Documents, { global, attachTo: document.body })
    await flushPromises()
    expect(w.find('[data-test="doc-row-d1"]').exists()).toBe(true)
    expect(w.find('[data-test="doc-proc-d1"]').text()).toBe('completed')
    await w.find('[data-test=doc-upload]').trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector('[role=dialog]')!
    ;(dialog.querySelector('[data-test=upload-submit]') as HTMLButtonElement).click()
    await flushPromises()
    expect(calls.some((c) => c.init.method === 'POST')).toBe(false)
    expect(dialog.querySelector('[role=alert]')?.textContent).toContain('Choose a file')
    const input = dialog.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', { value: [new File(['hello'], 'note.txt', { type: 'text/plain' })] })
    input.dispatchEvent(new Event('change'))
    ;(dialog.querySelector('[data-test=upload-submit]') as HTMLButtonElement).click()
    await flushPromises()
    const post = calls.find((c) => c.init.method === 'POST')!
    expect(post.url).toBe('/api/paperless/v1/documents')
    expect(post.init.body).toBeInstanceOf(FormData)
    expect((post.init.body as FormData).get('file')).toBeInstanceOf(File)
    expect((post.init.headers as Record<string, string>)['X-CSRF-Token']).toBe('tok')
    expect(calls.filter((c) => c.init.method === 'GET' && c.url.startsWith('/api/paperless/v1/documents')).length).toBe(2) // list reloaded after the upload
    // drawer
    await w.find('[data-test="doc-row-d1"]').trigger('click')
    await flushPromises()
    const drawer = document.body.querySelector('aside[role=dialog]')!
    expect(drawer.textContent).toContain('inv.pdf')
    const name = drawer.querySelector<HTMLInputElement>('input[data-field=name]')!
    expect(name.value).toBe('Invoice')
    name.value = 'Renamed'
    name.dispatchEvent(new Event('input'))
    ;(Array.from(drawer.querySelectorAll('button')).find((b) => b.textContent?.trim() === 'Save') as HTMLButtonElement).click()
    await flushPromises()
    const put = calls.find((c) => c.init.method === 'PUT')!
    expect(JSON.parse(String(put.init.body))).toEqual({ name: 'Renamed', tags: { year: '2026' } })
    w.unmount()
  })

  it('categories: tree with create form and selection panel', async () => {
    const calls = fetchMock((url, init) => (init.method === 'POST' ? { ...cat, id: 'c2', name: 'Tax' } : url.endsWith('/categories/tree') ? { tree: [{ category: cat, children: [] }] } : { items: [cat] }))
    const w = mount(Categories, { global })
    await flushPromises()
    expect(w.findAll('[role=treeitem]').length).toBe(1)
    await w.findAll('form')[0]!.trigger('submit')
    await flushPromises()
    expect(calls.some((c) => c.init.method === 'POST')).toBe(false)
    await w.find('input[data-field=name]').setValue('Tax')
    await w.findAll('form')[0]!.trigger('submit')
    await flushPromises()
    expect(JSON.parse(String(calls.find((c) => c.init.method === 'POST')!.init.body))).toEqual({ name: 'Tax' })
    await w.find('[role=treeitem]').trigger('click')
    expect(w.text()).toContain('/Finance')
    expect(w.find('[data-test=cat-move]').exists()).toBe(true)
    w.unmount()
  })

  it('search: validated query and result list', async () => {
    const calls = fetchMock(() => ({ items: [{ id: 'h1', name: 'Invoice', mime_type: 'application/pdf', rank: 0.5, snippet: 'total due' }] }))
    const w = mount(Search, { global })
    await w.find('input[data-field=query]').setValue('a')
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(calls.length).toBe(0)
    await w.find('input[data-field=query]').setValue('invoice')
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(JSON.parse(String(calls[0]!.init.body))).toEqual({ query: 'invoice', limit: 25 })
    expect(w.find('[data-test=hit-h1]').text()).toContain('total due')
    w.unmount()
  })

  it('dashboard: tiles and bars from the snapshot', async () => {
    fetchMock((url) => (url.includes('/statistics') ? { documents_total: 4, categories_total: 1, storage_bytes: 1048576, documents_by_status: { active: 4 }, documents_by_mime: { 'application/pdf': 4 }, backlog: { pending: 1 } } : { items: [] }))
    const w = mount(Dashboard, { global })
    await flushPromises()
    expect(w.findAll('.stat-tile').length).toBe(4)
    expect(w.text()).toContain('1.0 MB')
    expect(w.findAll('progress').length).toBe(3)
    w.unmount()
  })
})
