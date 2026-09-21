<script setup lang="ts">
import { ref } from 'vue'
import { useDocuments } from '@/stores/documents'
import { describe } from '@/api/client'
import type { SearchHit } from '@/api/types'

const store = useDocuments()
const query = ref('')
const hits = ref<SearchHit[]>([])
const searched = ref(false)
const busy = ref(false)
const error = ref('')

async function run(): Promise<void> {
  const q = query.value.trim()
  if (!q) return
  busy.value = true
  error.value = ''
  try {
    hits.value = await store.search(q, 25)
    searched.value = true
  } catch (e) {
    error.value = describe(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div>
    <h1 class="text-h5 mb-4">Search</h1>
    <v-text-field
      v-model="query"
      label="Search documents"
      density="comfortable"
      variant="outlined"
      prepend-inner-icon="mdi-magnify"
      clearable
      data-test="search-input"
      class="mb-4"
      @keyup.enter="run"
    >
      <template #append>
        <v-btn color="primary" :loading="busy" data-test="search-go" @click="run">Search</v-btn>
      </template>
    </v-text-field>

    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>

    <v-card v-if="hits.length">
      <v-list lines="two" data-test="search-results">
        <v-list-item v-for="h in hits" :key="h.id" :data-test="'hit-' + h.id">
          <template #prepend>
            <v-icon icon="mdi-file-document-outline" />
          </template>
          <v-list-item-title>{{ h.name }}</v-list-item-title>
          <v-list-item-subtitle>
            <span v-if="h.category_path" class="text-medium-emphasis me-2">{{ h.category_path }}</span>
            <!-- snippet may contain server-highlighted markup; rendered as text -->
            <span v-if="h.snippet">— {{ h.snippet }}</span>
          </v-list-item-subtitle>
          <template #append>
            <v-chip size="x-small" variant="tonal" class="me-2">{{ h.mime_type }}</v-chip>
            <v-chip size="x-small" variant="text">rank {{ h.rank.toFixed(2) }}</v-chip>
          </template>
        </v-list-item>
      </v-list>
    </v-card>
    <div v-else-if="searched && !busy" class="text-medium-emphasis">No matches.</div>
  </div>
</template>
