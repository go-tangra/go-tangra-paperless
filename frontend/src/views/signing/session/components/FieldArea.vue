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
        ? 'z-20 border-2 border-blue-500 bg-blue-50/70 ring-2 ring-blue-300 dark:bg-blue-900/40 dark:ring-blue-700'
        : isFilled
          ? 'z-10 border-2 border-green-400 bg-green-50/60 dark:bg-green-900/30'
          : 'z-10 border-2 border-orange-400 bg-orange-50/60 hover:bg-orange-100/70 dark:bg-orange-900/30 dark:hover:bg-orange-800/40',
      isActive && !isFilled ? 'animate-pulse-subtle' : '',
    ]"
    :style="style"
    style="min-width: 60px; min-height: 24px"
    @click.stop="emit('click')"
  >
    <div class="flex w-full items-center gap-1 px-1.5 py-0.5">
      <template v-if="isFilled && displayValue">
        <span
          class="truncate text-xs font-medium text-green-700 dark:text-green-300"
        >
          {{ displayValue }}
        </span>
      </template>
      <template v-else>
        <span class="flex-shrink-0 text-xs">{{ fieldIcon }}</span>
        <span
          class="truncate text-xs"
          :class="
            isActive
              ? 'font-medium text-blue-700 dark:text-blue-300'
              : 'text-orange-700 dark:text-orange-300'
          "
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
</style>
