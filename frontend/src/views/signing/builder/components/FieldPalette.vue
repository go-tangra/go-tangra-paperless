<script lang="ts" setup>
import {
  LucideType,
  LucidePenTool,
  LucideCalendar,
  LucidePenLine,
  LucideSquareCheck,
  LucideMail,
} from 'shell/vben/icons';

import { $t } from 'shell/locales';

defineEmits<{
  (e: 'drag-start', type: string, event: DragEvent): void;
}>();

const fieldTypes = [
  { type: 'text', label: $t('paperless.page.signingTemplate.typeText'), icon: LucideType },
  { type: 'signature', label: $t('paperless.page.signingTemplate.typeSignature'), icon: LucidePenTool },
  { type: 'date', label: $t('paperless.page.signingTemplate.typeDate'), icon: LucideCalendar },
  { type: 'initials', label: $t('paperless.page.signingTemplate.typeInitials'), icon: LucidePenLine },
  { type: 'checkbox', label: $t('paperless.page.signingTemplate.typeCheckbox'), icon: LucideSquareCheck },
  { type: 'email', label: $t('paperless.page.signingTemplate.typeEmail'), icon: LucideMail },
];
</script>

<template>
  <div class="space-y-2">
    <p class="text-xs font-medium uppercase tracking-wide text-gray-500">
      {{ $t('paperless.page.builder.dragToPlace') }}
    </p>
    <div
      v-for="ft in fieldTypes"
      :key="ft.type"
      class="palette-item flex cursor-grab items-center gap-2 rounded-lg border p-2.5 transition-colors active:cursor-grabbing"
      draggable="true"
      @dragstart="$emit('drag-start', ft.type, $event)"
    >
      <component :is="ft.icon" class="size-4 text-gray-500" />
      <span class="text-sm font-medium">{{ ft.label }}</span>
    </div>
  </div>
</template>

<style scoped>
.palette-item {
  border-color: #e5e7eb;
  background-color: #fff;
}
.palette-item:hover {
  border-color: #60a5fa;
  background-color: #eff6ff;
}
:global(.dark) .palette-item {
  border-color: #4b5563;
  background-color: #1f2937;
}
:global(.dark) .palette-item:hover {
  border-color: #3b82f6;
  background-color: #374151;
}
</style>
