<script lang="ts" setup>
import { computed } from 'vue';

import { Button, Input, DatePicker, Checkbox, Progress } from 'ant-design-vue';
import dayjs from 'dayjs';

import type { FlowField } from '../composables/useSigningFlow';
import SignatureCanvas from './SignatureCanvas.vue';

const props = defineProps<{
  activeField: FlowField | undefined;
  activeIndex: number;
  totalFields: number;
  filledCount: number;
  canSubmit: boolean;
  isLastField: boolean;
  submitting: boolean;
}>();

const emit = defineEmits<{
  next: [];
  previous: [];
  submit: [];
  'update:value': [value: string];
  'update:signature': [value: string | undefined];
}>();

const isSignatureField = computed(() => {
  const t = props.activeField?.field.type;
  return t === 'SIGNING_FIELD_TYPE_SIGNATURE' || t === 'signature';
});

const isDateField = computed(() => {
  const t = props.activeField?.field.type;
  return t === 'SIGNING_FIELD_TYPE_DATE' || t === 'date';
});

const isCheckboxField = computed(() => {
  const t = props.activeField?.field.type;
  return t === 'SIGNING_FIELD_TYPE_CHECKBOX' || t === 'checkbox';
});

const progressPercent = computed(() => {
  if (props.totalFields === 0) return 0;
  return Math.round((props.filledCount / props.totalFields) * 100);
});
</script>

<template>
  <div
    class="sticky bottom-0 z-30 border-t bg-white shadow-[0_-4px_12px_rgba(0,0,0,0.08)] dark:bg-gray-800"
  >
    <!-- Progress bar -->
    <div class="px-4 pt-3">
      <div class="flex items-center justify-between text-xs text-gray-500">
        <span>
          Field {{ activeIndex + 1 }} of {{ totalFields }}
        </span>
        <span>{{ filledCount }} / {{ totalFields }} completed</span>
      </div>
      <Progress
        :percent="progressPercent"
        :show-info="false"
        size="small"
        class="!mb-0"
        :stroke-color="progressPercent === 100 ? '#52c41a' : '#1890ff'"
      />
    </div>

    <!-- Field input area -->
    <div v-if="activeField" class="px-4 py-3">
      <div class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ activeField.field.name }}
        <span v-if="activeField.field.required" class="text-red-500">*</span>
      </div>

      <!-- Signature input -->
      <div v-if="isSignatureField" class="max-h-[220px] overflow-y-auto">
        <SignatureCanvas
          :model-value="activeField.signatureImage"
          @update:model-value="emit('update:signature', $event)"
        />
      </div>

      <!-- Date input -->
      <DatePicker
        v-else-if="isDateField"
        :value="activeField.value ? dayjs(activeField.value) : undefined"
        style="width: 100%"
        size="large"
        :placeholder="activeField.field.name"
        @change="(_date: any, dateString: any) => emit('update:value', Array.isArray(dateString) ? dateString[0] ?? '' : dateString ?? '')"
      />

      <!-- Checkbox input -->
      <Checkbox
        v-else-if="isCheckboxField"
        :checked="activeField.value === 'true'"
        @change="
          (e: any) => emit('update:value', e.target.checked ? 'true' : 'false')
        "
      >
        {{ activeField.field.name }}
      </Checkbox>

      <!-- Text input (default) -->
      <Input
        v-else
        :value="activeField.value"
        size="large"
        :placeholder="
          activeField.field.name
        "
        @update:value="emit('update:value', $event)"
      />
    </div>

    <!-- Navigation buttons -->
    <div class="flex items-center justify-between px-4 pb-3">
      <Button :disabled="activeIndex === 0" @click="emit('previous')">
        Previous
      </Button>
      <div class="flex gap-2">
        <Button
          v-if="!isLastField"
          type="primary"
          @click="emit('next')"
        >
          Next
        </Button>
        <Button
          v-if="isLastField || canSubmit"
          type="primary"
          :disabled="!canSubmit"
          :loading="submitting"
          @click="emit('submit')"
        >
          Sign Document
        </Button>
      </div>
    </div>
  </div>
</template>
