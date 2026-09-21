<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useDocuments } from '@/stores/documents'
import { useLive } from '@/stores/live'
import DocumentDrawer from '@/components/DocumentDrawer.vue'
import UploadDialog from '@/components/UploadDialog.vue'
import type { Document } from '@/api/types'

const store = useDocuments()
const live = useLive()
const drawer = ref(false)
const uploadOpen = ref(false)
const selected = ref<Document | null>(null)
const status = ref<string | null>(null)
const proc = ref<string | null>(null)

let release: (() => void) | null = null

onMounted(() => {
  void store.list()
  release = live.connect()
})
onUnmounted(() => release?.())

function reload(): void {
  void store.list({ status: status.value ?? undefined, processing_status: proc.value ?? undefined })
}

function open(d: Document): void {
  selected.value = d
  drawer.value = true
}

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
const STATUSES = ['active', 'archived', 'deleted']
const PROC = ['pending', 'processing', 'completed', 'failed', 'retrying']

function humanSize(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return (bytes / Math.pow(1024, i)).toFixed(i ? 1 : 0) + ' ' + units[i]
}
</script>

<template>
  <div>
    <div class="d-flex align-center mb-4">
      <h1 class="text-h5">Documents</h1>
      <v-chip v-if="live.connected" size="x-small" color="success" variant="tonal" class="ms-3">live</v-chip>
      <v-spacer />
      <v-select v-model="status" :items="STATUSES" label="Status" density="compact" clearable hide-details style="max-width: 150px" class="me-2" @update:model-value="reload" />
      <v-select v-model="proc" :items="PROC" label="Processing" density="compact" clearable hide-details style="max-width: 160px" class="me-2" @update:model-value="reload" />
      <v-btn variant="text" icon="mdi-refresh" class="me-2" @click="reload" />
      <v-btn color="primary" prepend-icon="mdi-upload" data-test="doc-upload" @click="uploadOpen = true">Upload</v-btn>
    </div>
    <v-alert v-if="store.error" type="error" variant="tonal" density="compact" class="mb-3">{{ store.error }}</v-alert>
    <v-table data-test="documents-table">
      <thead>
        <tr><th>Name</th><th>Type</th><th>Size</th><th>Status</th><th>Processing</th><th>Created</th></tr>
      </thead>
      <tbody>
        <tr v-for="d in store.items" :key="d.id" class="cursor-pointer" :data-test="'doc-row-' + d.id" @click="open(d)">
          <td>{{ d.name }}</td>
          <td class="text-medium-emphasis">{{ d.mime_type }}</td>
          <td class="text-medium-emphasis">{{ humanSize(d.file_size) }}</td>
          <td><v-chip size="x-small" :color="statusColor[d.status]" variant="flat">{{ d.status }}</v-chip></td>
          <td><v-chip size="x-small" :color="procColor[d.processing_status]" variant="tonal" :data-test="'doc-proc-' + d.id">{{ d.processing_status }}</v-chip></td>
          <td class="text-medium-emphasis">{{ new Date(d.created_at).toLocaleString() }}</td>
        </tr>
        <tr v-if="!store.items.length && !store.loading">
          <td colspan="6" class="text-medium-emphasis">No documents yet.</td>
        </tr>
      </tbody>
    </v-table>
    <DocumentDrawer v-model="drawer" :document="selected" @changed="reload" />
    <UploadDialog v-model="uploadOpen" @uploaded="reload" />
  </div>
</template>

<style scoped>
.cursor-pointer { cursor: pointer; }
</style>
