<script setup lang="ts">
import { ref } from 'vue'
import type { CategoryTreeNode } from '@/api/types'

defineOptions({ name: 'CategoryNode' })

defineProps<{ node: CategoryTreeNode; depth: number }>()
const emit = defineEmits<{ select: [CategoryTreeNode]; addChild: [CategoryTreeNode] }>()

const expanded = ref(true)
</script>

<template>
  <div>
    <div class="d-flex align-center py-1" :style="{ paddingLeft: depth * 16 + 'px' }">
      <v-btn
        v-if="node.children.length"
        :icon="expanded ? 'mdi-chevron-down' : 'mdi-chevron-right'"
        size="x-small"
        variant="text"
        @click="expanded = !expanded"
      />
      <v-icon v-else size="small" class="mx-2" icon="mdi-folder-outline" />
      <span class="cursor-pointer text-body-2" :data-test="'cat-' + node.category.id" @click="emit('select', node)">{{ node.category.name }}</span>
      <v-chip size="x-small" variant="tonal" class="ms-2">{{ node.category.document_count }}</v-chip>
      <v-spacer />
      <v-btn size="x-small" variant="text" icon="mdi-plus" :title="'Add subcategory'" @click="emit('addChild', node)" />
    </div>
    <template v-if="expanded">
      <CategoryNode
        v-for="child in node.children"
        :key="child.category.id"
        :node="child"
        :depth="depth + 1"
        @select="emit('select', $event)"
        @add-child="emit('addChild', $event)"
      />
    </template>
  </div>
</template>

<style scoped>
.cursor-pointer { cursor: pointer; }
</style>
