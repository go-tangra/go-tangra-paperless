<script lang="ts" setup>
import { computed } from 'vue';

import type { FlowField } from '../composables/useSigningFlow';

const props = defineProps<{
  flowField: FlowField;
  isActive: boolean;
  isFilled: boolean;
}>();

const emit = defineEmits<{
  click: [];
}>();

const style = computed(() => ({
  left: `${props.flowField.position?.xPercent ?? 0}%`,
  top: `${props.flowField.position?.yPercent ?? 0}%`,
  width: `${Math.max(props.flowField.position?.widthPercent ?? 10, 8)}%`,
  minHeight: `${Math.max(props.flowField.position?.heightPercent ?? 3, 2.4)}%`,
}));

const fieldIcon = computed(() => {
  const t = props.flowField.field.type;
  if (t === 'SIGNING_FIELD_TYPE_SIGNATURE' || t === 'signature') return '\u270D';
  if (t === 'SIGNING_FIELD_TYPE_DATE' || t === 'date') return '\uD83D\uDCC5';
  if (t === 'SIGNING_FIELD_TYPE_CHECKBOX' || t === 'checkbox') return '\u2611';
  return 'Aa';
});

const displayValue = computed(() => {
  const f = props.flowField;
  const isSignature =
    f.field.type === 'SIGNING_FIELD_TYPE_SIGNATURE' ||
    f.field.type === 'signature';
  if (isSignature && f.signatureImage) return '[Signed]';
  if (f.value) return f.value;
  return null;
});
</script>

<template>
  <div
    class="absolute flex cursor-pointer items-center transition-all duration-200"
    :class="[
      isActive
        ? 'field-active z-20 border-2'
        : isFilled
          ? 'field-filled z-10 border-2'
          : 'field-empty z-10 border-2',
      isActive && !isFilled ? 'animate-pulse-subtle' : '',
    ]"
    :style="style"
    style="min-width: 60px; min-height: 24px"
    @click.stop="emit('click')"
  >
    <div class="flex w-full items-center gap-1 px-1.5 py-0.5">
      <template v-if="isFilled && displayValue">
        <span
          class="field-value-text truncate text-xs font-medium"
        >
          {{ displayValue }}
        </span>
      </template>
      <template v-else>
        <span class="flex-shrink-0 text-xs">{{ fieldIcon }}</span>
        <span
          class="truncate text-xs"
          :class="isActive ? 'field-label-active font-medium' : 'field-label-empty'"
        >
          {{ flowField.field.name }}
        </span>
      </template>
    </div>
  </div>
</template>

<style scoped>
@keyframes pulse-subtle {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.75;
  }
}
.animate-pulse-subtle {
  animation: pulse-subtle 2s ease-in-out infinite;
}
.field-active {
  border-color: #3b82f6;
  background-color: rgb(239 246 255 / 0.7);
  box-shadow: 0 0 0 2px #93c5fd;
}
.field-filled {
  border-color: #4ade80;
  background-color: rgb(240 253 244 / 0.6);
}
.field-empty {
  border-color: #fb923c;
  background-color: rgb(255 247 237 / 0.6);
}
.field-empty:hover {
  background-color: rgb(255 237 213 / 0.7);
}
.field-value-text {
  color: #15803d;
}
.field-label-active {
  color: #1d4ed8;
}
.field-label-empty {
  color: #c2410c;
}
</style>
