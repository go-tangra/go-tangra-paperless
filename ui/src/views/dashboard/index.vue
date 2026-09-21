<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useDocuments } from '@/stores/documents'
import { useCategories } from '@/stores/categories'
import { useStats } from '@/stores/stats'
import StatsCard from '@/components/StatsCard.vue'

// The dashboard prefers the /statistics/tenant snapshot; when it is unavailable
// it derives figures from the loaded document and category lists.
const documents = useDocuments()
const categories = useCategories()
const stats = useStats()

onMounted(() => {
  void documents.list()
  void categories.list()
  void stats.load()
})

const snap = computed(() => stats.snapshot)

const documentsTotal = computed(() => snap.value?.documents_total ?? documents.items.length)
const categoriesTotal = computed(() => snap.value?.categories_total ?? categories.items.length)

function humanSize(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return (bytes / Math.pow(1024, i)).toFixed(i ? 1 : 0) + ' ' + units[i]
}

const storage = computed(() => humanSize(snap.value?.storage_bytes ?? 0))

const byStatus = computed<Record<string, number>>(() => {
  if (snap.value) return snap.value.documents_by_status
  const m: Record<string, number> = {}
  for (const d of documents.items) m[d.status] = (m[d.status] ?? 0) + 1
  return m
})

const byMime = computed<[string, number][]>(() => {
  const src = snap.value ? snap.value.documents_by_mime : deriveMime()
  return Object.entries(src).sort((a, b) => b[1] - a[1])
})

function deriveMime(): Record<string, number> {
  const m: Record<string, number> = {}
  for (const d of documents.items) m[d.mime_type] = (m[d.mime_type] ?? 0) + 1
  return m
}

const backlog = computed<[string, number][]>(() => {
  const src = snap.value ? snap.value.backlog : deriveBacklog()
  return Object.entries(src).sort((a, b) => b[1] - a[1])
})

function deriveBacklog(): Record<string, number> {
  const m: Record<string, number> = {}
  for (const d of documents.items) {
    if (d.processing_status !== 'completed') m[d.processing_status] = (m[d.processing_status] ?? 0) + 1
  }
  return m
}

const backlogTotal = computed(() => backlog.value.reduce((s, [, n]) => s + n, 0))

const statusMax = computed(() => Math.max(1, ...Object.values(byStatus.value)))
const mimeMax = computed(() => Math.max(1, ...byMime.value.map(([, n]) => n)))

const statusColor: Record<string, string> = {
  active: 'success',
  archived: 'grey',
  deleted: 'error',
}
const procColor: Record<string, string> = {
  pending: 'grey',
  processing: 'info',
  failed: 'error',
  retrying: 'warning',
}
</script>

<template>
  <div>
    <h1 class="text-h5 mb-4">Paperless</h1>
    <v-row>
      <v-col cols="12" sm="6" md="3">
        <StatsCard title="Documents" :value="documentsTotal" icon="mdi-file-document-multiple-outline" color="primary" />
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <StatsCard title="Categories" :value="categoriesTotal" icon="mdi-folder-outline" color="info" />
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <StatsCard title="Storage used" :value="storage" icon="mdi-database-outline" color="teal" />
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <StatsCard title="Processing backlog" :value="backlogTotal" icon="mdi-progress-clock" color="warning" subtitle="not completed" />
      </v-col>
    </v-row>

    <v-row class="mt-2">
      <v-col cols="12" md="6">
        <v-card>
          <v-card-title class="text-subtitle-1">Documents by status</v-card-title>
          <v-card-text>
            <div v-for="(n, s) in byStatus" :key="s" class="d-flex align-center mb-2">
              <v-chip size="x-small" :color="statusColor[s]" variant="flat" class="me-3" style="min-width: 92px; justify-content: center">{{ s }}</v-chip>
              <v-progress-linear :model-value="(n / statusMax) * 100" height="8" rounded :color="statusColor[s]" />
              <span class="ms-3 text-body-2">{{ n }}</span>
            </div>
            <div v-if="!Object.keys(byStatus).length" class="text-medium-emphasis">No documents yet.</div>
          </v-card-text>
        </v-card>
      </v-col>
      <v-col cols="12" md="6">
        <v-card>
          <v-card-title class="text-subtitle-1">Documents by type</v-card-title>
          <v-card-text>
            <div v-for="[mime, n] in byMime" :key="mime" class="d-flex align-center mb-2">
              <v-chip size="x-small" variant="tonal" class="me-3" style="min-width: 120px; justify-content: center">{{ mime }}</v-chip>
              <v-progress-linear :model-value="(n / mimeMax) * 100" height="8" rounded color="primary" />
              <span class="ms-3 text-body-2">{{ n }}</span>
            </div>
            <div v-if="!byMime.length" class="text-medium-emphasis">No documents yet.</div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-row v-if="backlog.length" class="mt-2">
      <v-col cols="12" md="6">
        <v-card>
          <v-card-title class="text-subtitle-1">Processing backlog</v-card-title>
          <v-card-text>
            <div v-for="[st, n] in backlog" :key="st" class="d-flex align-center mb-2">
              <v-chip size="x-small" :color="procColor[st]" variant="tonal" class="me-3" style="min-width: 92px; justify-content: center">{{ st }}</v-chip>
              <span class="ms-3 text-body-2">{{ n }}</span>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>
