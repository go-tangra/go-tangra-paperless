<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useCategories } from '@/stores/categories'
import { describe } from '@/api/client'
import CategoryNode from '@/components/CategoryNode.vue'
import type { CategoryTreeNode } from '@/api/types'

const store = useCategories()
const selected = ref<CategoryTreeNode | null>(null)
const error = ref('')
const busy = ref(false)

// Create form
const newName = ref('')
const newParent = ref<string | null>(null)

// Move form (targets the selected category)
const moveParent = ref<string | null>(null)

const parentOptions = computed(() => [
  { title: '(root)', value: null },
  ...store.items.map((c) => ({ title: c.path || c.name, value: c.id })),
])

async function reload(): Promise<void> {
  await Promise.all([store.list(), store.tree()])
}

onMounted(reload)

function select(node: CategoryTreeNode): void {
  selected.value = node
  moveParent.value = node.category.parent_id
}

function addChild(node: CategoryTreeNode): void {
  newParent.value = node.category.id
  newName.value = ''
}

async function create(): Promise<void> {
  if (!newName.value.trim()) return
  busy.value = true
  error.value = ''
  try {
    await store.create({ name: newName.value.trim(), parent_id: newParent.value ?? undefined })
    newName.value = ''
    newParent.value = null
    await reload()
  } catch (e) {
    error.value = describe(e)
  } finally {
    busy.value = false
  }
}

async function move(): Promise<void> {
  if (!selected.value) return
  busy.value = true
  error.value = ''
  try {
    await store.move(selected.value.category.id, moveParent.value ?? '')
    await reload()
    selected.value = null
  } catch (e) {
    error.value = describe(e)
  } finally {
    busy.value = false
  }
}

async function remove(): Promise<void> {
  if (!selected.value) return
  busy.value = true
  error.value = ''
  try {
    await store.remove(selected.value.category.id)
    await reload()
    selected.value = null
  } catch (e) {
    error.value = describe(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div>
    <div class="d-flex align-center mb-4">
      <h1 class="text-h5">Categories</h1>
      <v-spacer />
      <v-btn variant="text" icon="mdi-refresh" @click="reload" />
    </div>
    <v-alert v-if="error || store.error" type="error" variant="tonal" density="compact" class="mb-3">{{ error || store.error }}</v-alert>
    <v-row>
      <v-col cols="12" md="7">
        <v-card>
          <v-card-title class="text-subtitle-1">Tree</v-card-title>
          <v-card-text>
            <CategoryNode
              v-for="node in store.nodes"
              :key="node.category.id"
              :node="node"
              :depth="0"
              @select="select"
              @add-child="addChild"
            />
            <div v-if="!store.nodes.length && !store.loading" class="text-medium-emphasis">No categories yet.</div>
          </v-card-text>
        </v-card>
      </v-col>
      <v-col cols="12" md="5">
        <v-card class="mb-4">
          <v-card-title class="text-subtitle-1">New category</v-card-title>
          <v-card-text>
            <v-text-field v-model="newName" label="Name" density="compact" hide-details class="mb-3" data-test="cat-new-name" />
            <v-select v-model="newParent" :items="parentOptions" label="Parent" density="compact" hide-details class="mb-3" />
            <v-btn color="primary" :loading="busy" data-test="cat-create" @click="create">Create</v-btn>
          </v-card-text>
        </v-card>
        <v-card v-if="selected">
          <v-card-title class="text-subtitle-1">{{ selected.category.name }}</v-card-title>
          <v-card-text>
            <v-table density="compact" class="mb-3">
              <tbody>
                <tr><td class="text-medium-emphasis">Path</td><td>{{ selected.category.path }}</td></tr>
                <tr><td class="text-medium-emphasis">Documents</td><td>{{ selected.category.document_count }}</td></tr>
                <tr><td class="text-medium-emphasis">Subcategories</td><td>{{ selected.category.subcategory_count }}</td></tr>
                <tr><td class="text-medium-emphasis">Depth</td><td>{{ selected.category.depth }}</td></tr>
              </tbody>
            </v-table>
            <v-select v-model="moveParent" :items="parentOptions" label="Move to parent" density="compact" hide-details class="mb-3" />
            <div class="d-flex ga-2">
              <v-btn color="primary" variant="tonal" :loading="busy" data-test="cat-move" @click="move">Move</v-btn>
              <v-spacer />
              <v-btn color="error" variant="tonal" :loading="busy" @click="remove">Delete</v-btn>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>
