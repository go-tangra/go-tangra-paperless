import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '@/api/client'
import type { Category, CategoryInput, CategoryTreeNode } from '@/api/types'

export const useCategories = defineStore('paperless-categories', () => {
  const items = ref<Category[]>([])
  const nodes = ref<CategoryTreeNode[]>([])
  const loading = ref(false)
  const error = ref('')

  async function list(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const res = await api<{ items: Category[] }>('GET', 'categories')
      items.value = res.items ?? []
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  // tree loads the tenant's category forest (GetTree) into `nodes`.
  async function tree(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const res = await api<{ tree: CategoryTreeNode[] }>('GET', 'categories/tree')
      nodes.value = res.tree ?? []
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  async function create(input: CategoryInput): Promise<Category> {
    const c = await api<Category>('POST', 'categories', input)
    items.value = [...items.value, c]
    return c
  }

  async function update(id: string, input: CategoryInput): Promise<Category> {
    const c = await api<Category>('PUT', 'categories/' + id, input)
    items.value = items.value.map((x) => (x.id === id ? c : x))
    return c
  }

  async function remove(id: string): Promise<void> {
    await api('POST', 'categories/' + id + '/remove')
    items.value = items.value.filter((x) => x.id !== id)
  }

  async function move(id: string, parentId: string): Promise<Category> {
    const c = await api<Category>('POST', 'categories/' + id + '/move', { parent_id: parentId })
    items.value = items.value.map((x) => (x.id === id ? c : x))
    return c
  }

  return { items, nodes, loading, error, list, tree, create, update, remove, move }
})
