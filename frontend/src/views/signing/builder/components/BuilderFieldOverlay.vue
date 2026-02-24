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
    case 'signature': return 'border-red-400 bg-red-50/80 dark:bg-red-900/30';
    case 'text': return 'border-blue-400 bg-blue-50/80 dark:bg-blue-900/30';
    case 'date': return 'border-green-400 bg-green-50/80 dark:bg-green-900/30';
    case 'initials': return 'border-orange-400 bg-orange-50/80 dark:bg-orange-900/30';
    case 'checkbox': return 'border-cyan-400 bg-cyan-50/80 dark:bg-cyan-900/30';
    case 'email': return 'border-purple-400 bg-purple-50/80 dark:bg-purple-900/30';
    default: return 'border-gray-400 bg-gray-50/80 dark:bg-gray-900/30';
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
    :class="[typeColor, selected ? 'ring-2 ring-blue-500 shadow-lg z-20' : 'z-10']"
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
        class="absolute -bottom-1 -right-1 h-3 w-3 cursor-se-resize rounded-sm bg-blue-500"
        @mousedown="onResizeStart"
      />
    </template>
  </div>
</template>
