import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import SharingDrawer from '@/components/SharingDrawer.vue'
import Categories from '@/views/categories/index.vue'
import { ABILITY_TOKEN } from '@casl/vue'
import { createMongoAbility } from '@casl/ability'
import { createRouter, createMemoryHistory } from 'vue-router'
import { grantable, heldRelation } from '@/api/types'

const all = { read: true, write: true, delete: true, share: true, download: true }

describe('relation helpers', () => {
  it('reconstructs the held relation from the effective flags', () => {
    expect(heldRelation(all)).toBe('owner')
    expect(heldRelation({ ...all, write: false, delete: false, download: false })).toBe('sharer')
    expect(heldRelation({ ...all, share: false })).toBe('editor')
    expect(heldRelation({ read: true, write: false, delete: false, share: false, download: true })).toBe('viewer')
    expect(heldRelation({ read: false, write: false, delete: false, share: false, download: false })).toBe('')
    expect(heldRelation(null)).toBe('')
  })
  it('never offers a level above the held one', () => {
    expect(grantable('owner')).toEqual(['viewer', 'sharer', 'editor', 'owner'])
    expect(grantable('sharer')).toEqual(['viewer', 'sharer'])
    expect(grantable('')).toEqual([])
  })
})

describe('sharing drawer', () => {
  const grant = { id: 'g1', resource_type: 'category', resource_id: 'c1', subject_type: 'user', subject_id: 'u2', relation: 'viewer' }
  let calls: { url: string; init: RequestInit }[]
  function mockFetch(perms: typeof all) {
    calls = []
    vi.stubGlobal('fetch', vi.fn(async (url: string, init: RequestInit = {}) => {
      calls.push({ url, init })
      let body: unknown = {}
      if (url.includes('/permissions/effective')) body = { permissions: perms, grants: [] }
      else if (url.includes('/permissions/grant')) body = { ...grant, id: 'g2', subject_type: 'role', subject_id: 'member', relation: 'editor' }
      else if (url.includes('/permissions/revoke')) body = {}
      else if (url.includes('/api/paperless/v1/permissions')) body = { items: [grant] }
      else if (url.includes('/api/v1/roles')) body = [{ slug: 'member', display_name: 'Member' }]
      else if (url.includes('/api/v1/users/lookup')) body = { items: [{ id: 'u2', display_name: 'Maria' }] }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }))
  }
  beforeEach(() => {
    setActivePinia(createPinia())
    document.cookie = '__Host-csrf=tok; Secure; Path=/'
    document.body.innerHTML = ''
  })

  it('lists grants with resolved names and loads the caller relation and roles', async () => {
    mockFetch(all)
    mount(SharingDrawer, { props: { resourceType: 'category', resourceId: 'c1', name: 'Finance', modelValue: true }, attachTo: document.body })
    await flushPromises()
    const text = document.body.textContent ?? ''
    expect(text).toContain('Access to Finance')
    expect(text).toContain('Maria')
    expect(text).toContain('Your relation: owner')
    expect(text).toContain('also apply to every document')
    expect(calls.some((c) => c.url.includes('resource_type=category') && c.url.includes('resource_id=c1'))).toBe(true)
    expect(calls.some((c) => c.url.includes('/api/v1/roles'))).toBe(true)
    const revoke = document.body.querySelector<HTMLButtonElement>('button[aria-label="Revoke"]')!
    expect(revoke).not.toBeNull()
    revoke.click()
    await flushPromises()
    const rv = calls.find((c) => c.url.includes('/permissions/revoke'))!
    expect(JSON.parse(String(rv.init.body))).toEqual({ resource_type: 'category', resource_id: 'c1', id: 'g1' })
    expect(document.body.textContent).not.toContain('Maria')
  })

  it('does nothing while closed', async () => {
    mockFetch(all)
    mount(SharingDrawer, { props: { resourceType: 'document', resourceId: 'd1', name: 'Invoice', modelValue: false }, attachTo: document.body })
    await flushPromises()
    expect(calls.length).toBe(0)
  })

  it('a viewer sees the grants but cannot manage them', async () => {
    mockFetch({ read: true, write: false, delete: false, share: false, download: true })
    mount(SharingDrawer, { props: { resourceType: 'document', resourceId: 'd1', name: 'Invoice', modelValue: true }, attachTo: document.body })
    await flushPromises()
    const text = document.body.textContent ?? ''
    expect(text).toContain('Your relation: viewer')
    expect(document.body.querySelector('button[aria-label="Revoke"]')).toBeNull()
  })
})

describe('share access entry points', () => {
  const cat = { id: 'c1', parent_id: null, name: 'Finance', path: '/Finance', depth: 0, sort_order: 0, document_count: 1, subcategory_count: 0 }
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div/>' } }] })
  beforeEach(() => {
    setActivePinia(createPinia())
    document.cookie = '__Host-csrf=tok; Secure; Path=/'
    document.body.innerHTML = ''
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      const body = url.endsWith('/categories/tree') ? { tree: [{ category: cat, children: [] }] } : url.includes('/permissions/effective') ? { permissions: { read: true, write: true, delete: true, share: true, download: true }, grants: [] } : url.includes('/api/v1/roles') ? [] : { items: url.includes('/categories') ? [cat] : [] }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }))
  })
  async function mountWith(rules: { action: string; subject: string }[]) {
    const w = mount(Categories, { global: { plugins: [router], provide: { [ABILITY_TOKEN as symbol]: createMongoAbility(rules) } }, attachTo: document.body })
    await flushPromises()
    await w.find('[role=treeitem]').trigger('click')
    await flushPromises()
    return w
  }

  it('a paperless administrator opens the category access drawer', async () => {
    const w = await mountWith([{ action: 'manage', subject: 'PaperlessPermission' }, { action: 'read', subject: 'all' }])
    await w.find('[data-test=cat-share]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('Access to Finance')
    w.unmount()
  })

  it('is hidden without permissions:manage', async () => {
    const w = await mountWith([{ action: 'read', subject: 'all' }])
    expect(w.find('[data-test=cat-move]').exists()).toBe(true)
    expect(w.find('[data-test=cat-share]').exists()).toBe(false)
    w.unmount()
  })
})
