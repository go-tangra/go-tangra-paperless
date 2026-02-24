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
      class="flex cursor-grab items-center gap-2 rounded-lg border border-gray-200 bg-white p-2.5 transition-colors hover:border-blue-400 hover:bg-blue-50 active:cursor-grabbing dark:border-gray-600 dark:bg-gray-800 dark:hover:border-blue-500 dark:hover:bg-gray-700"
      draggable="true"
      @dragstart="$emit('drag-start', ft.type, $event)"
    >
      <component :is="ft.icon" class="size-4 text-gray-500" />
      <span class="text-sm font-medium">{{ ft.label }}</span>
    </div>
  </div>
</template>
