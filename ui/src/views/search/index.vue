<script setup lang="ts">
import { ref } from 'vue'
import { UiPage, UiAlert, UiCard, UiForm, UiInput, UiButton, UiBadge, UiIcon, UiEmptyState } from '@go-tangra/ui'
import { useZodForm } from '@go-tangra/ui/forms'
import { useDocuments } from '@/stores/documents'
import { searchSchema } from '@/schemas'
import type { SearchHit } from '@/api/types'

const store = useDocuments()
const hits = ref<SearchHit[]>([])
const searched = ref(false)
const form = useZodForm(searchSchema, {
  initial: { query: '' },
  onSubmit: async (v) => {
    hits.value = await store.search(v.query, 25)
    searched.value = true
  },
})
</script>

<template>
  <UiPage title="Search">
    <UiForm :form="form" class="mb-4">
      <div class="flex flex-wrap items-end gap-2">
        <UiInput v-bind="form.field('query')" label="Search documents" type="search" class="w-full md:max-w-xl" placeholder="Words from the title or the extracted text" data-test="search-input" @enter="form.submit()" />
        <UiButton type="submit" icon="mdi-magnify" :loading="form.submitting.value" data-test="search-go">Search</UiButton>
      </div>
    </UiForm>
    <UiAlert v-if="form.serverError.value" kind="error" class="mb-3">{{ form.serverError.value }}</UiAlert>
    <UiCard v-if="hits.length" :padded="false">
      <ul class="divide-y divide-base-300" data-test="search-results">
        <li v-for="h in hits" :key="h.id" class="flex items-start gap-3 px-4 py-3" :data-test="'hit-' + h.id">
          <UiIcon name="mdi-file-document-outline" class="mt-0.5 shrink-0 text-base-content/70" />
          <div class="min-w-0 grow">
            <div class="truncate font-medium">{{ h.name }}</div>
            <div class="text-sm text-base-content/70"><span v-if="h.category_path" class="me-2">{{ h.category_path }}</span><span v-if="h.snippet">— {{ h.snippet }}</span></div>
          </div>
          <div class="flex shrink-0 flex-col items-end gap-1"><UiBadge size="xs">{{ h.mime_type }}</UiBadge><span class="text-xs text-base-content/70">rank {{ h.rank.toFixed(2) }}</span></div>
        </li>
      </ul>
    </UiCard>
    <UiEmptyState v-else-if="searched && !form.submitting.value" title="No matches" icon="mdi-file-search-outline" />
  </UiPage>
</template>
