<script lang="ts" setup>
import { LucideGripVertical } from 'shell/vben/icons';

import { Tag } from 'ant-design-vue';

import { $t } from 'shell/locales';

import type { BuilderField } from '../composables/useFieldBuilder';
import { useFieldBuilder } from '../composables/useFieldBuilder';

const { fieldTypeToShort } = useFieldBuilder();

defineProps<{
  fields: BuilderField[];
  selectedFieldId: string | null;
}>();

defineEmits<{
  (e: 'select', id: string): void;
}>();

function typeColor(field: BuilderField): string {
  switch (fieldTypeToShort(field.type)) {
    case 'signature': return 'red';
    case 'text': return 'blue';
    case 'date': return 'green';
    case 'initials': return 'orange';
    case 'checkbox': return 'cyan';
    case 'email': return 'purple';
    default: return 'default';
  }
}

function typeLabel(field: BuilderField): string {
  switch (fieldTypeToShort(field.type)) {
    case 'text': return $t('paperless.page.signingTemplate.typeText');
    case 'signature': return $t('paperless.page.signingTemplate.typeSignature');
    case 'date': return $t('paperless.page.signingTemplate.typeDate');
    case 'initials': return $t('paperless.page.signingTemplate.typeInitials');
    case 'checkbox': return $t('paperless.page.signingTemplate.typeCheckbox');
    case 'email': return $t('paperless.page.signingTemplate.typeEmail');
    default: return '-';
  }
}
</script>

<template>
  <div class="mt-4">
    <p class="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500">
      {{ $t('paperless.page.signingTemplate.fields') }} ({{ fields.length }})
    </p>
    <div v-if="fields.length === 0" class="text-center text-xs text-gray-400 py-4">
      {{ $t('paperless.page.builder.noFields') }}
    </div>
    <div class="space-y-1">
      <div
        v-for="field in fields"
        :key="field.id"
        class="field-list-item flex cursor-pointer items-center gap-1.5 rounded px-2 py-1.5 text-xs transition-colors"
        :class="{ 'field-list-item-selected': field.id === selectedFieldId }"
        @click="$emit('select', field.id)"
      >
        <LucideGripVertical class="size-3 text-gray-400" />
        <Tag :color="typeColor(field)" class="m-0 text-[10px]">{{ typeLabel(field) }}</Tag>
        <span class="flex-1 truncate">{{ field.name }}</span>
        <span class="text-[10px] text-gray-400">p{{ field.pageNumber }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.field-list-item:hover {
  background-color: #f3f4f6;
}
.field-list-item-selected {
  background-color: #dbeafe;
}
:global(.dark) .field-list-item:hover {
  background-color: #374151;
}
:global(.dark) .field-list-item-selected {
  background-color: rgb(30 58 138 / 0.4);
}
</style>
