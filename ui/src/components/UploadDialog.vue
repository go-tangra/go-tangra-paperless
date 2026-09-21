<script setup lang="ts">
import { ref, watch } from 'vue'
import { useDocuments } from '@/stores/documents'
import { useCategories } from '@/stores/categories'
import { describe } from '@/api/client'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; uploaded: [] }>()

const store = useDocuments()
const categories = useCategories()

// v-file-input's single-file model is File | null.
const file = ref<File | null>(null)
const name = ref('')
const description = ref('')
const categoryId = ref<string | null>(null)
const tagsText = ref('')
const error = ref('')
const busy = ref(false)

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return
    void categories.list()
    file.value = null
    name.value = ''
    description.value = ''
    categoryId.value = null
    tagsText.value = ''
    error.value = ''
  },
)

function parseTags(): Record<string, string> | undefined {
  const raw = tagsText.value.trim()
  if (!raw) return undefined
  try {
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object') return parsed as Record<string, string>
  } catch {
    /* invalid JSON dropped */
  }
  return undefined
}

async function submit(): Promise<void> {
  if (!file.value) {
    error.value = 'Choose a file to upload.'
    return
  }
  busy.value = true
  error.value = ''
  try {
    await store.upload({
      file: file.value,
      name: name.value.trim() || undefined,
      description: description.value.trim() || undefined,
      category_id: categoryId.value ?? undefined,
      tags: parseTags(),
    })
    emit('uploaded')
    emit('update:modelValue', false)
  } catch (e) {
    error.value = describe(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <v-dialog :model-value="modelValue" max-width="520" @update:model-value="emit('update:modelValue', $event)">
    <v-card>
      <v-card-title class="text-subtitle-1">Upload document</v-card-title>
      <v-card-text>
        <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3">{{ error }}</v-alert>
        <v-file-input v-model="file" label="File" density="compact" prepend-icon="mdi-paperclip" show-size data-test="upload-file" class="mb-2" />
        <v-text-field v-model="name" label="Name (optional)" density="compact" hide-details class="mb-3" />
        <v-textarea v-model="description" label="Description (optional)" density="compact" rows="2" auto-grow hide-details class="mb-3" />
        <v-select v-model="categoryId" :items="categories.items.map((c) => ({ title: c.path || c.name, value: c.id }))" label="Category (optional)" density="compact" clearable hide-details class="mb-3" />
        <v-text-field v-model="tagsText" label="Tags (JSON, optional)" density="compact" hide-details placeholder='{"key":"value"}' />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="emit('update:modelValue', false)">Cancel</v-btn>
        <v-btn color="primary" :loading="busy" data-test="upload-submit" @click="submit">Upload</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
