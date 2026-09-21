<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDocuments } from '@/stores/documents'
import { useCategories } from '@/stores/categories'
import { describe } from '@/api/client'
import type { Document, DocumentInput } from '@/api/types'

const props = defineProps<{ modelValue: boolean; document: Document | null }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; changed: [] }>()

const store = useDocuments()
const categories = useCategories()

const name = ref('')
const description = ref('')
const categoryId = ref<string | null>(null)
const tagsText = ref('')
const error = ref('')
const busy = ref(false)
const downloadUrl = ref('')

const doc = computed(() => props.document)

const procColor: Record<string, string> = {
  completed: 'success',
  failed: 'error',
  processing: 'info',
  retrying: 'warning',
  pending: 'grey',
}
const statusColor: Record<string, string> = {
  active: 'success',
  archived: 'grey',
  deleted: 'error',
}

const categoryOptions = computed(() => categories.items.map((c) => ({ title: c.path || c.name, value: c.id })))

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return
    void categories.list()
    error.value = ''
    downloadUrl.value = ''
    const d = props.document
    name.value = d?.name ?? ''
    description.value = d?.description ?? ''
    categoryId.value = d?.category_id ?? null
    tagsText.value = d?.tags ? JSON.stringify(d.tags) : ''
  },
)

function tags(): Record<string, string> | undefined {
  const raw = tagsText.value.trim()
  if (!raw) return undefined
  try {
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object') return parsed as Record<string, string>
  } catch {
    /* invalid JSON is dropped; the field guards it below */
  }
  return undefined
}

async function save(): Promise<void> {
  if (!props.document) return
  busy.value = true
  error.value = ''
  try {
    const input: DocumentInput = {
      name: name.value.trim(),
      description: description.value.trim() || undefined,
      category_id: categoryId.value ?? undefined,
      tags: tags(),
    }
    await store.update(props.document.id, input)
    emit('changed')
  } catch (e) {
    error.value = describe(e)
  } finally {
    busy.value = false
  }
}

async function remove(): Promise<void> {
  if (!props.document) return
  busy.value = true
  error.value = ''
  try {
    await store.remove(props.document.id)
    emit('changed')
    emit('update:modelValue', false)
  } catch (e) {
    error.value = describe(e)
  } finally {
    busy.value = false
  }
}

async function fetchLink(): Promise<void> {
  if (!props.document) return
  error.value = ''
  try {
    downloadUrl.value = await store.downloadUrl(props.document.id)
  } catch (e) {
    error.value = describe(e)
  }
}

function humanSize(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return (bytes / Math.pow(1024, i)).toFixed(i ? 1 : 0) + ' ' + units[i]
}
</script>

<template>
  <v-navigation-drawer :model-value="modelValue" location="right" temporary width="500" @update:model-value="emit('update:modelValue', $event)">
    <v-toolbar density="comfortable" title="Document">
      <v-btn icon="mdi-close" variant="text" @click="emit('update:modelValue', false)" />
    </v-toolbar>
    <div v-if="doc" class="pa-4">
      <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>

      <div class="d-flex align-center mb-3">
        <v-chip :color="statusColor[doc.status]" size="small" variant="flat">{{ doc.status }}</v-chip>
        <v-chip class="ms-2" size="small" variant="tonal">{{ doc.source }}</v-chip>
        <v-chip class="ms-2" size="small" :color="procColor[doc.processing_status]" variant="tonal" :data-test="'proc-' + doc.id">
          {{ doc.processing_status }}
        </v-chip>
      </div>

      <v-text-field v-model="name" label="Name" density="compact" hide-details class="mb-3" />
      <v-textarea v-model="description" label="Description" density="compact" rows="2" auto-grow hide-details class="mb-3" />
      <v-select v-model="categoryId" :items="categoryOptions" label="Category" density="compact" clearable hide-details class="mb-3" />
      <v-text-field v-model="tagsText" label="Tags (JSON)" density="compact" hide-details placeholder='{"key":"value"}' class="mb-3" />

      <v-table density="compact" class="mb-3">
        <tbody>
          <tr><td class="text-medium-emphasis">File</td><td>{{ doc.file_name }}</td></tr>
          <tr><td class="text-medium-emphasis">Size</td><td>{{ humanSize(doc.file_size) }}</td></tr>
          <tr><td class="text-medium-emphasis">Type</td><td>{{ doc.mime_type }}</td></tr>
          <tr><td class="text-medium-emphasis">Checksum</td><td class="text-truncate" style="max-width: 260px">{{ doc.checksum }}</td></tr>
          <tr v-if="doc.category_path"><td class="text-medium-emphasis">Path</td><td>{{ doc.category_path }}</td></tr>
          <tr><td class="text-medium-emphasis">Created by</td><td>{{ doc.created_by }}</td></tr>
          <tr><td class="text-medium-emphasis">Created</td><td>{{ new Date(doc.created_at).toLocaleString() }}</td></tr>
        </tbody>
      </v-table>

      <div v-if="doc.tags && Object.keys(doc.tags).length" class="mb-3">
        <div class="text-subtitle-2 mb-1">Tags</div>
        <v-chip v-for="(v, k) in doc.tags" :key="k" size="x-small" variant="tonal" class="me-1 mb-1">{{ k }}: {{ v }}</v-chip>
      </div>

      <div class="d-flex flex-wrap ga-2 mb-3">
        <v-btn size="small" variant="tonal" :href="store.directDownload(doc.id)" target="_blank" prepend-icon="mdi-download">Download</v-btn>
        <v-btn size="small" variant="text" prepend-icon="mdi-link-variant" @click="fetchLink">Get link</v-btn>
      </div>
      <v-alert v-if="downloadUrl" type="info" variant="tonal" density="compact" class="mb-3">
        <a :href="downloadUrl" target="_blank" class="text-truncate d-inline-block" style="max-width: 420px">{{ downloadUrl }}</a>
      </v-alert>

      <div class="d-flex ga-2">
        <v-btn color="primary" :loading="busy" data-test="doc-save" @click="save">Save</v-btn>
        <v-spacer />
        <v-btn color="error" variant="tonal" :loading="busy" @click="remove">Delete</v-btn>
      </div>
    </div>
  </v-navigation-drawer>
</template>
