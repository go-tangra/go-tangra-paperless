<script lang="ts" setup>
import { ref, shallowRef, onMounted, watch, nextTick } from 'vue';

import { Spin } from 'ant-design-vue';
import { GlobalWorkerOptions, getDocument } from 'pdfjs-dist';
import type { PDFDocumentProxy } from 'pdfjs-dist';
import PdfWorker from 'pdfjs-dist/build/pdf.worker.min.mjs?url';

import type { BuilderField } from '../composables/useFieldBuilder';

import BuilderFieldOverlay from './BuilderFieldOverlay.vue';

GlobalWorkerOptions.workerSrc = PdfWorker;

const props = defineProps<{
  pdfUrl: string;
  fields: BuilderField[];
  selectedFieldId: string | null;
}>();

const emit = defineEmits<{
  (e: 'field-select', id: string): void;
  (e: 'field-move', id: string, xPercent: number, yPercent: number): void;
  (e: 'field-resize', id: string, widthPercent: number, heightPercent: number): void;
  (e: 'drop-field', type: string, pageNumber: number, xPercent: number, yPercent: number): void;
  (e: 'deselect'): void;
}>();

const loading = ref(true);
const pdfDoc = shallowRef<PDFDocumentProxy | null>(null);
const pages = ref<Array<{ num: number; canvas: HTMLCanvasElement | null; width: number; height: number }>>([]);
const containerRefs = new Map<number, HTMLElement>();
const RENDER_SCALE = 1.5;

function setContainerRef(idx: number, el: any) {
  if (el) {
    containerRefs.set(idx, el as HTMLElement);
  } else {
    containerRefs.delete(idx);
  }
}

async function loadPdf(url: string) {
  loading.value = true;
  try {
    const doc = await getDocument(url).promise;
    pdfDoc.value = doc;
    const pageList: typeof pages.value = [];
    for (let i = 1; i <= doc.numPages; i++) {
      const page = await doc.getPage(i);
      const viewport = page.getViewport({ scale: 1 });
      pageList.push({ num: i, canvas: null, width: viewport.width, height: viewport.height });
    }
    pages.value = pageList;
    loading.value = false;
    await nextTick();
    for (let i = 0; i < pageList.length; i++) {
      await renderPage(i);
    }
  } catch {
    loading.value = false;
  }
}

async function renderPage(index: number) {
  if (!pdfDoc.value) return;
  const pageInfo = pages.value[index];
  if (!pageInfo) return;
  const page = await pdfDoc.value.getPage(pageInfo.num);
  const viewport = page.getViewport({ scale: RENDER_SCALE });
  const canvas = document.createElement('canvas');
  canvas.width = viewport.width;
  canvas.height = viewport.height;
  canvas.style.width = '100%';
  canvas.style.height = 'auto';
  const ctx = canvas.getContext('2d')!;
  await page.render({ canvasContext: ctx, viewport }).promise;
  const container = containerRefs.get(index);
  if (container) {
    const existing = container.querySelector('canvas');
    if (existing) existing.remove();
    container.prepend(canvas);
  }
}

function fieldsOnPage(pageNum: number) {
  return props.fields.filter((f) => f.pageNumber === pageNum);
}

function getPageElement(pageNum: number): HTMLElement | null {
  return containerRefs.get(pageNum - 1) ?? null;
}

function onPageDragOver(e: DragEvent) {
  e.preventDefault();
  if (e.dataTransfer) {
    e.dataTransfer.dropEffect = 'copy';
  }
}

function onPageDrop(e: DragEvent, pageNum: number) {
  e.preventDefault();
  const type = e.dataTransfer?.getData('field-type');
  if (!type) return;
  const el = getPageElement(pageNum);
  if (!el) return;
  const rect = el.getBoundingClientRect();
  const xPercent = ((e.clientX - rect.left) / rect.width) * 100;
  const yPercent = ((e.clientY - rect.top) / rect.height) * 100;
  emit('drop-field', type, pageNum, Math.max(0, xPercent), Math.max(0, yPercent));
}

function onPageClick(e: MouseEvent) {
  if ((e.target as HTMLElement).closest('.builder-field-overlay')) return;
  emit('deselect');
}

onMounted(() => {
  if (props.pdfUrl) {
    loadPdf(props.pdfUrl);
  }
});

watch(() => props.pdfUrl, (url) => {
  if (url) loadPdf(url);
});
</script>

<template>
  <div class="flex flex-col items-center gap-4">
    <div v-if="loading" class="flex items-center justify-center py-20">
      <Spin size="large" tip="Loading PDF..." />
    </div>
    <template v-else>
      <div
        v-for="(page, idx) in pages"
        :key="page.num"
        :ref="(el) => setContainerRef(idx, el)"
        class="relative w-full border bg-white shadow-md dark:border-gray-600"
        @dragover="onPageDragOver"
        @drop="onPageDrop($event, page.num)"
        @click="onPageClick"
      >
        <div class="absolute left-2 top-2 z-30 rounded bg-black/50 px-2 py-0.5 text-xs text-white">
          Page {{ page.num }}
        </div>
        <BuilderFieldOverlay
          v-for="field in fieldsOnPage(page.num)"
          :key="field.id"
          class="builder-field-overlay"
          :field="field"
          :selected="field.id === selectedFieldId"
          :page-width="containerRefs.get(idx)?.clientWidth ?? 600"
          :page-height="containerRefs.get(idx)?.clientHeight ?? 800"
          @select="$emit('field-select', field.id)"
          @move="(x, y) => $emit('field-move', field.id, x, y)"
          @resize="(w, h) => $emit('field-resize', field.id, w, h)"
        />
      </div>
    </template>
  </div>
</template>
