<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useDocuments } from '@/stores/documents'
import { useCategories } from '@/stores/categories'
import { useStats } from '@/stores/stats'
import { UiPage, UiCard, UiStatGrid, UiStatTile, UiBarList, type BarItem } from '@go-tangra/ui'

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
const statusColor: Record<string, NonNullable<BarItem['color']>> = { active: 'success', archived: 'neutral', deleted: 'error' }
const procColor: Record<string, NonNullable<BarItem['color']>> = { pending: 'neutral', processing: 'info', failed: 'error', retrying: 'warning' }
const statusBars = computed<BarItem[]>(() => Object.entries(byStatus.value).sort((a, b) => b[1] - a[1]).map(([label, value]) => ({ label, value, color: statusColor[label] ?? 'primary' })))
const mimeBars = computed<BarItem[]>(() => byMime.value.map(([label, value]) => ({ label, value, color: 'primary' })))
const backlogBars = computed<BarItem[]>(() => backlog.value.map(([label, value]) => ({ label, value, color: procColor[label] ?? 'warning' })))
</script>
<template>
  <UiPage title="Paperless">
    <UiStatGrid class="mb-4" :cols="4">
      <UiStatTile title="Documents" :value="documentsTotal" icon="mdi-file-document-multiple-outline" color="primary" />
      <UiStatTile title="Categories" :value="categoriesTotal" icon="mdi-folder-outline" color="info" />
      <UiStatTile title="Storage used" :value="storage" icon="mdi-database-outline" color="accent" />
      <UiStatTile title="Processing backlog" :value="backlogTotal" icon="mdi-progress-clock" color="warning" subtitle="not completed" />
    </UiStatGrid>
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <UiCard title="Documents by status"><UiBarList :items="statusBars" empty-title="No documents yet" /></UiCard>
      <UiCard title="Documents by type"><UiBarList :items="mimeBars" empty-title="No documents yet" /></UiCard>
      <UiCard v-if="backlog.length" title="Processing backlog"><UiBarList :items="backlogBars" /></UiCard>
    </div>
  </UiPage>
</template>
