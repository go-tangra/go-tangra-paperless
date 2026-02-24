<script lang="ts" setup>
import { ref, computed } from 'vue';

import type { BuilderField } from '../composables/useFieldBuilder';
import { useFieldBuilder } from '../composables/useFieldBuilder';

const { fieldTypeToShort } = useFieldBuilder();

const props = defineProps<{
  field: BuilderField;
  selected: boolean;
  pageWidth: number;
  pageHeight: number;
}>();

const emit = defineEmits<{
  (e: 'select'): void;
  (e: 'move', xPercent: number, yPercent: number): void;
  (e: 'resize', widthPercent: number, heightPercent: number): void;
}>();

const dragging = ref(false);
const resizing = ref(false);
const startX = ref(0);
const startY = ref(0);
const startFieldX = ref(0);
const startFieldY = ref(0);
const startFieldW = ref(0);
const startFieldH = ref(0);

const style = computed(() => ({
  left: `${props.field.xPercent}%`,
  top: `${props.field.yPercent}%`,
  width: `${props.field.widthPercent}%`,
  height: `${props.field.heightPercent}%`,
}));

const typeShort = computed(() => fieldTypeToShort(props.field.type));

const typeColor = computed(() => {
  switch (typeShort.value) {
    case 'signature': return 'field-type-signature';
    case 'text': return 'field-type-text';
    case 'date': return 'field-type-date';
    case 'initials': return 'field-type-initials';
    case 'checkbox': return 'field-type-checkbox';
    case 'email': return 'field-type-email';
    default: return 'field-type-default';
  }
});

function onMouseDown(e: MouseEvent) {
  e.preventDefault();
  e.stopPropagation();
  emit('select');
  dragging.value = true;
  startX.value = e.clientX;
  startY.value = e.clientY;
  startFieldX.value = props.field.xPercent ?? 0;
  startFieldY.value = props.field.yPercent ?? 0;
  document.addEventListener('mousemove', onMouseMove);
  document.addEventListener('mouseup', onMouseUp);
}

function onMouseMove(e: MouseEvent) {
  if (!dragging.value && !resizing.value) return;
  const dx = e.clientX - startX.value;
  const dy = e.clientY - startY.value;
  const dxPercent = (dx / props.pageWidth) * 100;
  const dyPercent = (dy / props.pageHeight) * 100;

  if (dragging.value) {
    const newX = Math.max(0, Math.min(100 - (props.field.widthPercent ?? 10), startFieldX.value + dxPercent));
    const newY = Math.max(0, Math.min(100 - (props.field.heightPercent ?? 3), startFieldY.value + dyPercent));
    emit('move', newX, newY);
  } else if (resizing.value) {
    const newW = Math.max(2, Math.min(100 - (props.field.xPercent ?? 0), startFieldW.value + dxPercent));
    const newH = Math.max(1, Math.min(100 - (props.field.yPercent ?? 0), startFieldH.value + dyPercent));
    emit('resize', newW, newH);
  }
}

function onMouseUp() {
  dragging.value = false;
  resizing.value = false;
  document.removeEventListener('mousemove', onMouseMove);
  document.removeEventListener('mouseup', onMouseUp);
}

function onResizeStart(e: MouseEvent) {
  e.preventDefault();
  e.stopPropagation();
  emit('select');
  resizing.value = true;
  startX.value = e.clientX;
  startY.value = e.clientY;
  startFieldW.value = props.field.widthPercent ?? 10;
  startFieldH.value = props.field.heightPercent ?? 3;
  document.addEventListener('mousemove', onMouseMove);
  document.addEventListener('mouseup', onMouseUp);
}
</script>

<template>
  <div
    class="absolute cursor-move select-none border-2 transition-shadow"
    :class="[typeColor, selected ? 'field-selected z-20' : 'z-10']"
    :style="style"
    @mousedown="onMouseDown"
  >
    <div class="flex h-full items-center overflow-hidden px-1">
      <span class="truncate text-[10px] font-medium leading-tight text-gray-700 dark:text-gray-200">
        {{ field.name }}
      </span>
    </div>
    <!-- Resize handles (visible when selected) -->
    <template v-if="selected">
      <div
        class="absolute -bottom-1 -right-1 h-3 w-3 cursor-se-resize rounded-sm resize-handle"
        @mousedown="onResizeStart"
      />
    </template>
  </div>
</template>

<style scoped>
.field-selected {
  box-shadow: 0 0 0 2px #3b82f6, 0 10px 15px -3px rgb(0 0 0 / 0.1);
}
.resize-handle {
  background-color: #3b82f6;
}
.field-type-signature { border-color: #f87171; background-color: rgb(254 242 242 / 0.8); }
.field-type-text { border-color: #60a5fa; background-color: rgb(239 246 255 / 0.8); }
.field-type-date { border-color: #4ade80; background-color: rgb(240 253 244 / 0.8); }
.field-type-initials { border-color: #fb923c; background-color: rgb(255 247 237 / 0.8); }
.field-type-checkbox { border-color: #22d3ee; background-color: rgb(236 254 255 / 0.8); }
.field-type-email { border-color: #a78bfa; background-color: rgb(245 243 255 / 0.8); }
.field-type-default { border-color: #9ca3af; background-color: rgb(249 250 251 / 0.8); }

:global(.dark) .field-type-signature { background-color: rgb(127 29 29 / 0.3); }
:global(.dark) .field-type-text { background-color: rgb(30 58 138 / 0.3); }
:global(.dark) .field-type-date { background-color: rgb(20 83 45 / 0.3); }
:global(.dark) .field-type-initials { background-color: rgb(124 45 18 / 0.3); }
:global(.dark) .field-type-checkbox { background-color: rgb(22 78 99 / 0.3); }
:global(.dark) .field-type-email { background-color: rgb(76 29 149 / 0.3); }
:global(.dark) .field-type-default { background-color: rgb(17 24 39 / 0.3); }
</style>
