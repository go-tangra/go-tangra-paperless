<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { UiPage, UiAlert, UiCard, UiButton, UiTree, UiForm, UiInput, UiSelect, UiKeyValueTable, UiToolbar, useConfirm, type SelectOption, type TreeNode } from '@go-tangra/ui'
import { useZodForm } from '@go-tangra/ui/forms'
import { useCategories } from '@/stores/categories'
import { describe } from '@/api/client'
import { categorySchema, moveCategorySchema } from '@/schemas'
import type { CategoryTreeNode } from '@/api/types'

const store = useCategories()
const confirm = useConfirm()
const selectedId = ref('')
const error = ref('')

const parentOptions = computed<SelectOption[]>(() => store.items.map((c) => ({ title: c.path || c.name, value: c.id })))
const toNode = (n: CategoryTreeNode): TreeNode => ({ id: n.category.id, label: n.category.name, icon: 'mdi-folder-outline', badge: String(n.category.document_count), children: n.children.map(toNode) })
const tree = computed<TreeNode[]>(() => store.nodes.map(toNode))
const selected = computed(() => store.items.find((c) => c.id === selectedId.value) ?? null)

async function reload(): Promise<void> {
  await Promise.all([store.list(), store.tree()])
}
onMounted(reload)

const createForm = useZodForm(categorySchema, {
  initial: { name: '', parent_id: '' },
  onSubmit: (v) => store.create({ name: v.name, parent_id: v.parent_id }),
  onSuccess: async () => {
    createForm.reset({ name: '', parent_id: '' })
    await reload()
  },
})
const moveForm = useZodForm(moveCategorySchema, {
  initial: { parent_id: '' },
  onSubmit: (v) => store.move(selectedId.value, v.parent_id ?? ''),
  onSuccess: async () => {
    await reload()
    selectedId.value = ''
  },
})
function onSelect(n: TreeNode): void {
  moveForm.reset({ parent_id: store.items.find((c) => c.id === n.id)?.parent_id ?? '' })
}
function addChild(): void {
  createForm.reset({ name: '', parent_id: selectedId.value })
}
async function remove(): Promise<void> {
  if (!selected.value || !(await confirm.ask({ title: `Delete ${selected.value.name}?`, danger: true, confirmLabel: 'Delete' }))) return
  error.value = ''
  try {
    await store.remove(selected.value.id)
    selectedId.value = ''
    await reload()
  } catch (e) {
    error.value = describe(e)
  }
}
</script>

<template>
  <UiPage title="Categories">
    <template #actions><UiButton variant="text" icon="mdi-refresh" icon-only label="Refresh" @click="reload" /></template>
    <UiAlert v-if="error || store.error" kind="error" class="mb-3">{{ error || store.error }}</UiAlert>
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-12">
      <UiCard title="Tree" class="lg:col-span-7"><UiTree v-model:selected="selectedId" :items="tree" @select="onSelect" /></UiCard>
      <div class="flex flex-col gap-4 lg:col-span-5">
        <UiCard title="New category">
          <UiForm :form="createForm">
            <div class="flex flex-col gap-3">
              <UiInput v-bind="createForm.field('name')" label="Name" required data-test="cat-new-name" />
              <UiSelect v-bind="createForm.field('parent_id')" label="Parent" :options="parentOptions" placeholder="(root)" />
              <div><UiButton type="submit" :loading="createForm.submitting.value" data-test="cat-create">Create</UiButton></div>
            </div>
          </UiForm>
        </UiCard>
        <UiCard v-if="selected" :title="selected.name">
          <UiKeyValueTable class="mb-3" :items="[{ label: 'Path', value: selected.path }, { label: 'Documents', value: selected.document_count }, { label: 'Subcategories', value: selected.subcategory_count }, { label: 'Depth', value: selected.depth }]" />
          <UiForm :form="moveForm">
            <UiSelect v-bind="moveForm.field('parent_id')" label="Move to parent" :options="parentOptions.filter((o) => o.value !== selected!.id)" placeholder="(root)" />
            <UiToolbar class="mt-3">
              <UiButton type="submit" variant="soft" :loading="moveForm.submitting.value" data-test="cat-move">Move</UiButton>
              <UiButton variant="soft" icon="mdi-plus" @click="addChild">Add subcategory</UiButton>
              <span class="grow" />
              <UiButton variant="soft" color="error" @click="remove">Delete</UiButton>
            </UiToolbar>
          </UiForm>
        </UiCard>
      </div>
    </div>
  </UiPage>
</template>
