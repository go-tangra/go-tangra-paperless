<script setup lang="ts">
import { ABILITY_TOKEN } from '@casl/vue'
import type { AnyAbility } from '@casl/ability'
import { computed, inject, onMounted, onUnmounted, ref, watch } from 'vue'
import { UiPage, UiAlert, UiCard, UiForm, UiSelect, UiButton, UiDataTable, UiStatusChip, UiLiveIndicator, UiFilePicker, UiInput, UiTextarea, UiTagEditor, UiRecordDrawer, UiKeyValueTable, UiBadge, useConfirm, UiDrawer, useListQuery, useToast, type Column, type SelectOption } from '@go-tangra/ui'
import { useZodForm, zodToFields } from '@go-tangra/ui/forms'
import { DOCUMENT_LIST, useDocuments, type DocumentFilter } from '@/stores/documents'
import { useCategories } from '@/stores/categories'
import { useLive } from '@/stores/live'
import SharingDrawer from '@/components/SharingDrawer.vue'
import { describe } from '@/api/client'
import { documentFilterSchema, documentSchema, uploadSchema, DOCUMENT_STATUSES, PROCESSING_STATUSES, MAX_UPLOAD_BYTES } from '@/schemas'
import type { Document } from '@/api/types'

const store = useDocuments()
const categories = useCategories()
const live = useLive()
const confirm = useConfirm()
const toast = useToast()
const selected = ref<Document | null>(null)
const drawer = ref(false)
const uploadOpen = ref(false)
const shareOpen = ref(false)
// Listing and changing grants needs permissions:manage (ability manage PaperlessPermission).
const ability = inject<AnyAbility | null>(ABILITY_TOKEN, null)
const canShare = computed(() => (ability ? ability.can('manage', 'PaperlessPermission') : true))
const error = ref('')

// --- server paging and sorting (page / size / sort in the URL: ?documents.page=…) ---
const lq = useListQuery('documents', DOCUMENT_LIST)

let release: (() => void) | null = null
onMounted(() => {
  void load()
  void categories.list()
  release = live.connect()
})
onUnmounted(() => release?.())

const statusOptions: SelectOption[] = DOCUMENT_STATUSES.map((s) => ({ title: s, value: s }))
const procOptions: SelectOption[] = PROCESSING_STATUSES.map((s) => ({ title: s, value: s }))
const categoryOptions = computed<SelectOption[]>(() => categories.items.map((c) => ({ title: c.path || c.name, value: c.id })))
let filterValue: DocumentFilter = {}
async function load(): Promise<void> {
  const res = await store.list(filterValue, lq.query.value)
  if (res?.page) lq.clampTo(res.page) // a page beyond the end answers the last page
}
watch(lq.query, () => void load())
/** Filters changed: back to page 1 (which reloads), or reload in place. */
const filter = useZodForm(documentFilterSchema, {
  onSubmit: (f) => {
    const next: DocumentFilter = { status: f.status, processing_status: f.processing_status }
    const changed = next.status !== filterValue.status || next.processing_status !== filterValue.processing_status
    filterValue = next
    if (changed && lq.page.value !== 1) lq.resetPage()
    else void load()
  },
})
/** Reloads the current page (after an upload, edit or delete, or on Refresh). */
const reload = () => void filter.submit()

const procColors = { completed: 'success', failed: 'error', processing: 'info', retrying: 'warning', pending: 'neutral' } as const
const statusColors = { archived: 'neutral', deleted: 'error' } as const
function humanSize(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return (bytes / Math.pow(1024, i)).toFixed(i ? 1 : 0) + ' ' + units[i]
}
// Every column is one of the server's sort fields (DOCUMENT_LIST); sorting
// orders the whole list, not the visible page.
const columns: Column<Document>[] = [
  { key: 'name', label: 'Name', sortable: true },
  { key: 'mime_type', label: 'Type', sortable: true, hideOnStack: true },
  { key: 'file_size', label: 'Size', align: 'end', format: (d) => humanSize(d.file_size), sortable: true, defaultDir: 'desc' },
  { key: 'status', label: 'Status', width: 'sm', sortable: true },
  { key: 'processing_status', label: 'Processing', width: 'sm', sortable: true },
  { key: 'created_at', label: 'Created', format: (d) => new Date(d.created_at).toLocaleString(), sortable: true, defaultDir: 'desc', hideOnStack: true },
]

// --- upload: one Zod schema drives the multipart payload ---
const uploadForm = useZodForm(uploadSchema, {
  initial: { tags: {} },
  onSubmit: (v) => store.upload({ file: v.file, name: v.name, description: v.description, category_id: v.category_id, tags: Object.keys(v.tags).length ? v.tags : undefined }),
  onSuccess: () => {
    uploadOpen.value = false
    toast.success('Uploaded')
    reload()
  },
})
function openUpload(): void {
  uploadForm.reset({ tags: {} })
  uploadOpen.value = true
}

// --- detail drawer: schema-driven edit form plus metadata ---
const editFields = computed(() => zodToFields(documentSchema, { category_id: { type: 'select', options: categoryOptions.value }, description: { type: 'textarea', cols: 12 }, tags: { type: 'tags', cols: 12 } }))
const editInitial = computed(() => (selected.value ? { name: selected.value.name, description: selected.value.description ?? '', category_id: selected.value.category_id ?? '', tags: selected.value.tags ?? {} } : {}))
function open(d: Document): void {
  selected.value = d
  error.value = ''
  drawer.value = true
}
const save = (v: Record<string, unknown>) => store.update(selected.value!.id, v)
async function remove(): Promise<void> {
  if (!selected.value || !(await confirm.ask({ title: `Delete ${selected.value.name}?`, text: 'The document is moved to the deleted state.', danger: true, confirmLabel: 'Delete' }))) return
  try {
    await store.remove(selected.value.id)
    drawer.value = false
    reload()
  } catch (e) {
    error.value = describe(e)
  }
}
const meta = computed(() => {
  const d = selected.value
  if (!d) return []
  return [{ label: 'File', value: d.file_name }, { label: 'Size', value: humanSize(d.file_size) }, { label: 'Type', value: d.mime_type }, { label: 'Checksum', value: d.checksum, copyable: true }, { label: 'Path', value: d.category_path }, { label: 'Created by', value: d.created_by }, { label: 'Created', value: new Date(d.created_at).toLocaleString() }]
})
// One drawer at a time: the record drawer closes while access is managed.
function openShare(): void {
  drawer.value = false
  shareOpen.value = true
}
</script>

<template>
  <UiPage title="Documents">
    <template #badges><UiLiveIndicator :connected="live.connected" /></template>
    <template #actions>
      <UiButton variant="text" icon="mdi-refresh" icon-only label="Refresh" @click="reload" />
      <UiButton icon="mdi-upload" data-test="doc-upload" @click="openUpload">Upload</UiButton>
    </template>
    <template #filters>
      <UiForm :form="filter" class="w-full">
        <div class="grid grid-cols-2 gap-2 md:max-w-md">
          <UiSelect v-bind="filter.field('status')" label="Status" :options="statusOptions" size="sm" @update:model-value="reload" />
          <UiSelect v-bind="filter.field('processing_status')" label="Processing" :options="procOptions" size="sm" @update:model-value="reload" />
        </div>
      </UiForm>
    </template>
    <UiAlert v-if="store.error" kind="error" class="mb-3">{{ store.error }}</UiAlert>
    <UiCard :padded="false">
      <UiDataTable :items="store.items" :columns="columns" :loading="store.loading" :total="store.total" :page="lq.page.value" :page-size="lq.pageSize.value" :sort="lq.sort.value" caption="Documents" empty-title="No documents yet" clickable :row-attrs="(d) => ({ 'data-test': 'doc-row-' + d.id })" data-test="documents-table" @row-click="open" @update:page="lq.setPage" @update:page-size="lq.setPageSize" @update:sort="lq.setSort">
        <template #cell-status="{ row }"><UiStatusChip :status="row.status" :colors="statusColors" /></template>
        <template #cell-processing_status="{ row }"><UiStatusChip :status="row.processing_status" :colors="procColors" :data-test="'doc-proc-' + row.id" /></template>
      </UiDataTable>
    </UiCard>

    <UiDrawer v-model="uploadOpen" title="Upload document" size="md">
      <UiForm :form="uploadForm">
        <div class="flex flex-col gap-3">
          <UiFilePicker v-bind="uploadForm.field('file')" label="File" :max-bytes="MAX_UPLOAD_BYTES" required data-test="upload-file" />
          <UiInput v-bind="uploadForm.field('name')" label="Name (optional)" />
          <UiTextarea v-bind="uploadForm.field('description')" label="Description (optional)" :rows="2" />
          <UiSelect v-bind="uploadForm.field('category_id')" label="Category (optional)" :options="categoryOptions" />
          <UiTagEditor v-bind="uploadForm.field('tags')" label="Tags (optional)" :max="32" />
        </div>
      </UiForm>
      <template #actions>
        <UiButton variant="text" @click="uploadOpen = false">Cancel</UiButton>
        <UiButton :loading="uploadForm.submitting.value" data-test="upload-submit" @click="uploadForm.submit()">Upload</UiButton>
      </template>
    </UiDrawer>

    <UiRecordDrawer v-model="drawer" title="Document" :schema="documentSchema" :fields="editFields" :initial="editInitial" :submit="save" size="lg" @saved="reload">
      <template #before>
        <UiAlert v-if="error" kind="error" class="mb-3">{{ error }}</UiAlert>
        <div v-if="selected" class="mb-3 flex flex-wrap gap-1">
          <UiStatusChip :status="selected.status" :colors="statusColors" />
          <UiBadge>{{ selected.source }}</UiBadge>
          <UiStatusChip :status="selected.processing_status" :colors="procColors" :data-test="'proc-' + selected.id" />
        </div>
      </template>
      <template #after>
        <UiKeyValueTable class="mt-4" :items="meta" />
        <div class="mt-3 flex flex-wrap gap-2">
          <a v-if="selected" :href="store.directDownload(selected.id)" target="_blank" rel="noopener" class="btn btn-primary btn-soft btn-sm" data-test="doc-download"><span class="icon-[mdi--download] size-4" aria-hidden="true" />Download</a>
          <UiButton v-if="canShare" size="sm" variant="soft" icon="mdi-shield-account-outline" data-test="doc-share" @click="openShare">Share access</UiButton>
          <span class="grow" />
          <UiButton size="sm" variant="soft" color="error" icon="mdi-delete-outline" @click="remove">Delete</UiButton>
        </div>
      </template>
    </UiRecordDrawer>
    <SharingDrawer v-if="selected" v-model="shareOpen" resource-type="document" :resource-id="selected.id" :name="selected.name" />
  </UiPage>
</template>
