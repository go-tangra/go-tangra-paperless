<script setup lang="ts">
// Who can view, edit, share or own one document or category. Grants on a
// category apply to everything inside it; tenant admins always hold owner.
import { computed, watch } from 'vue'
import { UiPermissionDrawer, usePermissionGrants } from '@go-tangra/ui'
import { usePermissions } from '@/stores/permissions'
import { useDirectory } from '@/stores/directory'
import { grantable, heldRelation, type Relation, type ResourceType, type SubjectType } from '@/api/types'

const props = defineProps<{ resourceType: ResourceType; resourceId: string; name: string }>()
const open = defineModel<boolean>({ default: false })

const store = usePermissions()
const dir = useDirectory()

const access = usePermissionGrants({
  grants: () => store.items,
  effective: () => ({ relation: heldRelation(store.mine) || undefined, canShare: !!store.mine?.share }),
  grant: (r) => store.grant({ resource_type: props.resourceType, resource_id: props.resourceId, subject_type: r.subject_type as SubjectType, subject_id: r.subject_id ?? '', relation: r.relation as Relation, expires_at: r.expires_at ?? null }),
  revoke: (id) => store.revoke(props.resourceType, props.resourceId, id),
  directory: { roles: () => Object.values(dir.roles), searchUsers: dir.searchUsers, resolveUsers: dir.resolveUsers, userName: dir.userName, roleName: dir.roleName },
  grantable,
})

const hint = computed(() => (props.resourceType === 'category' ? 'Grants here also apply to every document and subcategory inside. ' : '') + access.hint.value)
const error = computed(() => access.error.value || store.error)

watch(
  () => [open.value, props.resourceType, props.resourceId] as const,
  async ([isOpen, type, id]) => {
    if (!isOpen || !id) return
    await Promise.all([store.load(type, id), dir.loadRoles()])
    await access.resolve()
  },
  { immediate: true },
)
</script>

<template>
  <UiPermissionDrawer v-model="open" :title="'Access to ' + name" :grants="access.grants.value" :subjects="access.subjects.value" :levels="access.levels.value" :can-manage="access.canShare.value" expires :editable-level="false" :hint="hint" :error="error" :busy="access.busy.value || store.loading" data-test="permission-drawer" @search="access.search" @grant="access.onGrant" @revoke="access.onRevoke" @change-level="access.onChangeLevel" />
</template>
