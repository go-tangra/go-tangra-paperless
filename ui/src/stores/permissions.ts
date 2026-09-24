import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '@/api/client'
import type { EffectivePermissions, GrantInput, PermissionTuple } from '@/api/types'

export const usePermissions = defineStore('paperless-permissions', () => {
  const items = ref<PermissionTuple[]>([])
  const loading = ref(false)
  const error = ref('')

  async function grant(input: GrantInput): Promise<PermissionTuple> {
    const t = await api<PermissionTuple>('POST', 'permissions/grant', input)
    items.value = [t, ...items.value]
    return t
  }

  async function revoke(resourceType: string, resourceId: string, id: string): Promise<void> {
    await api('POST', 'permissions/revoke', { resource_type: resourceType, resource_id: resourceId, id })
    items.value = items.value.filter((x) => x.id !== id)
  }

  async function list(resourceType: string, resourceId: string): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const res = await api<{ items: PermissionTuple[] }>('GET', 'permissions', undefined, {
        query: { resource_type: resourceType, resource_id: resourceId },
      })
      items.value = res.items ?? []
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  async function check(resourceType: string, resourceId: string, permission: string): Promise<boolean> {
    const res = await api<{ allowed: boolean }>('POST', 'permissions/check', {
      resource_type: resourceType,
      resource_id: resourceId,
      permission,
    })
    return res.allowed
  }

  async function effective(resourceType: string, resourceId: string): Promise<{ permissions: EffectivePermissions; grants: PermissionTuple[] }> {
    return api<{ permissions: EffectivePermissions; grants: PermissionTuple[] }>('GET', 'permissions/effective', undefined, {
      query: { resource_type: resourceType, resource_id: resourceId },
    })
  }

  return { items, loading, error, grant, revoke, list, check, effective }
})
