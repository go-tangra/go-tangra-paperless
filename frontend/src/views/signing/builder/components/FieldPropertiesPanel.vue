<script lang="ts" setup>
import { computed } from 'vue';

import { LucideTrash } from 'shell/vben/icons';

import {
  Input,
  Select,
  SelectOption,
  Checkbox,
  InputNumber,
  Button,
} from 'ant-design-vue';

import { $t } from 'shell/locales';
import type { SigningFieldType } from '../../../../stores/paperless-signing-template.state';

import type { BuilderField } from '../composables/useFieldBuilder';

const props = defineProps<{
  field: BuilderField;
}>();

const emit = defineEmits<{
  (e: 'update', updates: Partial<BuilderField>): void;
  (e: 'delete'): void;
}>();

const fieldTypeOptions = [
  { value: 'SIGNING_FIELD_TYPE_TEXT', label: $t('paperless.page.signingTemplate.typeText') },
  { value: 'SIGNING_FIELD_TYPE_SIGNATURE', label: $t('paperless.page.signingTemplate.typeSignature') },
  { value: 'SIGNING_FIELD_TYPE_DATE', label: $t('paperless.page.signingTemplate.typeDate') },
  { value: 'SIGNING_FIELD_TYPE_INITIALS', label: $t('paperless.page.signingTemplate.typeInitials') },
  { value: 'SIGNING_FIELD_TYPE_CHECKBOX', label: $t('paperless.page.signingTemplate.typeCheckbox') },
  { value: 'SIGNING_FIELD_TYPE_EMAIL', label: $t('paperless.page.signingTemplate.typeEmail') },
];

const pageNumber = computed(() => props.field.pageNumber ?? 1);
</script>

<template>
  <div class="space-y-3">
    <h3 class="text-sm font-semibold">
      {{ $t('paperless.page.builder.properties') }}
    </h3>

    <div>
      <label class="mb-1 block text-xs text-gray-500">{{ $t('paperless.page.signingTemplate.fieldName') }}</label>
      <Input
        :value="field.name"
        size="small"
        @update:value="(v: string) => emit('update', { name: v })"
      />
    </div>

    <div>
      <label class="mb-1 block text-xs text-gray-500">{{ $t('paperless.page.signingTemplate.fieldType') }}</label>
      <Select
        :value="field.type"
        size="small"
        style="width: 100%"
        @update:value="(v: SigningFieldType) => emit('update', { type: v })"
      >
        <SelectOption v-for="opt in fieldTypeOptions" :key="opt.value" :value="opt.value">
          {{ opt.label }}
        </SelectOption>
      </Select>
    </div>

    <div>
      <Checkbox
        :checked="field.required"
        @update:checked="(v: boolean) => emit('update', { required: v })"
      >
        {{ $t('paperless.page.signingTemplate.fieldRequired') }}
      </Checkbox>
    </div>

    <div>
      <label class="mb-1 block text-xs text-gray-500">{{ $t('paperless.page.builder.recipientIndex') }}</label>
      <InputNumber
        :value="field.recipientIndex ?? 0"
        :min="0"
        :max="20"
        size="small"
        style="width: 100%"
        @update:value="(v: number | null) => emit('update', { recipientIndex: v ?? 0 })"
      />
      <p class="mt-0.5 text-[10px] text-gray-400">0 = all recipients</p>
    </div>

    <div>
      <label class="mb-1 block text-xs text-gray-500">{{ $t('paperless.page.builder.prefillStage') }}</label>
      <InputNumber
        :value="field.prefillStage ?? 0"
        :min="0"
        :max="20"
        size="small"
        style="width: 100%"
        @update:value="(v: number | null) => emit('update', { prefillStage: v ?? 0 })"
      />
      <p class="mt-0.5 text-[10px] text-gray-400">0 = not prefilled</p>
    </div>

    <div class="rounded bg-gray-50 p-2 text-xs text-gray-500 dark:bg-gray-800">
      <p>Page: {{ pageNumber }}</p>
      <p>X: {{ (field.xPercent ?? 0).toFixed(1) }}%, Y: {{ (field.yPercent ?? 0).toFixed(1) }}%</p>
      <p>W: {{ (field.widthPercent ?? 0).toFixed(1) }}%, H: {{ (field.heightPercent ?? 0).toFixed(1) }}%</p>
    </div>

    <Button danger block size="small" @click="emit('delete')">
      <template #icon><LucideTrash class="size-3" /></template>
      {{ $t('paperless.page.builder.deleteField') }}
    </Button>
  </div>
</template>
