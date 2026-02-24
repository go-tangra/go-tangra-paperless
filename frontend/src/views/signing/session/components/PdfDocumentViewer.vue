<script lang="ts" setup>
import { computed, ref, watch, nextTick } from 'vue';

import { Spin } from 'ant-design-vue';
import { VuePDF, usePDF } from '@tato30/vue-pdf';

import type { FlowField } from '../composables/useSigningFlow';
import FieldArea from './FieldArea.vue';

const props = defineProps<{
  documentUrl: string;
  fields: FlowField[];
  activeFieldIndex: number;
  isFieldFilled: (f: FlowField) => boolean;
}>();

const emit = defineEmits<{
  'field-click': [index: number];
}>();

const { pdf, pages: pageCount } = usePDF(props.documentUrl);

const pageRefs = ref<Map<number, HTMLDivElement>>(new Map());

function setPageRef(pageNum: number, el: HTMLDivElement | null) {
  if (el) {
    pageRefs.value.set(pageNum, el);
  } else {
    pageRefs.value.delete(pageNum);
  }
}

function fieldsForPage(pageNum: number): Array<FlowField & { _index: number }> {
  return props.fields
    .map((f, index) => ({ ...f, _index: index }))
    .filter((f) => f.position?.pageNumber === pageNum);
}

// Scroll the active field's page into view
watch(
  () => props.activeFieldIndex,
  (newIndex) => {
    nextTick(() => {
      const field = props.fields[newIndex];
      if (!field?.position) return;

      const pageEl = pageRefs.value.get(field.position.pageNumber);
      if (!pageEl) return;

      pageEl.scrollIntoView({ behavior: 'smooth', block: 'center' });
    });
  },
);

// Generate page number array
const pageNumbers = computed(() => {
  const count = pageCount.value ?? 0;
  return Array.from({ length: count }, (_, i) => i + 1);
});
</script>

<template>
  <div class="pdf-viewer space-y-4">
    <div v-if="!pdf" class="flex items-center justify-center py-20">
      <Spin size="large" tip="Loading document..." />
    </div>
    <div
      v-for="pageNum in pageNumbers"
      v-else
      :key="pageNum"
      :ref="(el: any) => setPageRef(pageNum, el)"
      class="relative mx-auto shadow-lg"
    >
      <VuePDF :pdf="pdf" :page="pageNum" fit-parent />
      <!-- Field overlays -->
      <FieldArea
        v-for="field in fieldsForPage(pageNum)"
        :key="field.field.fieldId"
        :flow-field="field"
        :is-active="field._index === activeFieldIndex"
        :is-filled="isFieldFilled(field)"
        @click="emit('field-click', field._index)"
      />
    </div>
  </div>
</template>

<style scoped>
.pdf-viewer :deep(canvas) {
  width: 100% !important;
  height: auto !important;
}
</style>
