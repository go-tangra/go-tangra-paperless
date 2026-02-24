<script lang="ts" setup>
import { ref, onMounted, computed } from 'vue';
import { useRoute } from 'vue-router';

import {
  Button,
  Input,
  Checkbox,
  Spin,
  Result,
  notification,
  Card,
  DatePicker,
} from 'ant-design-vue';
import dayjs from 'dayjs';

import {
  getSigningSession,
  submitSigning,
  type SigningSessionResponse,
} from '../../../api/public-client';

import PdfDocumentViewer from './components/PdfDocumentViewer.vue';
import SignatureCanvas from './components/SignatureCanvas.vue';
import SigningPanel from './components/SigningPanel.vue';
import { useSigningFlow, type FlowField } from './composables/useSigningFlow';

const route = useRoute();
const token = computed(() => route.params.token as string);

// Page state
const loading = ref(true);
const submitting = ref(false);
const session = ref<SigningSessionResponse | null>(null);
const error = ref<string | null>(null);
const success = ref(false);

// Composables
const {
  fields,
  activeFieldIndex,
  activeField,
  totalFields,
  progress,
  hasPositions,
  canSubmit,
  isLastField,
  isFieldFilled,
  initializeFields,
  goToNext,
  goToPrevious,
  goToField,
  setFieldValue,
  setSignatureImage,
  buildSubmitPayload,
} = useSigningFlow();

// Layout mode
const layoutMode = computed<'overlay' | 'sidebar'>(() =>
  hasPositions.value ? 'overlay' : 'sidebar',
);

onMounted(async () => {
  if (!token.value) {
    error.value = 'Invalid signing link';
    loading.value = false;
    return;
  }

  try {
    session.value = await getSigningSession(token.value);

    // Initialize the signing flow
    initializeFields(session.value.fields ?? []);
  } catch (e: any) {
    error.value = e?.message || 'Failed to load signing session';
  } finally {
    loading.value = false;
  }
});

function handleFieldValueUpdate(value: string) {
  setFieldValue(activeFieldIndex.value, value);
}

function handleSignatureUpdate(value: string | undefined) {
  setSignatureImage(activeFieldIndex.value, value);
}

function handleFieldClick(index: number) {
  goToField(index);
}

async function handleSubmit() {
  // Validate required fields
  for (const f of fields.value) {
    if (f.field.required && !isFieldFilled(f)) {
      notification.error({
        message: 'Required field',
        description: `Please fill in "${f.field.name}"`,
      });
      // Jump to the unfilled required field
      const idx = fields.value.indexOf(f);
      if (idx >= 0) goToField(idx);
      return;
    }
  }

  submitting.value = true;
  try {
    const payload = buildSubmitPayload();
    await submitSigning(token.value, payload);
    success.value = true;
  } catch (e: any) {
    notification.error({
      message: 'Signing failed',
      description: e?.message || 'Failed to submit signing',
    });
  } finally {
    submitting.value = false;
  }
}

// --- Sidebar fallback helpers ---
function getFieldComponent(
  field: FlowField,
): 'checkbox' | 'date' | 'input' | 'signature' {
  const t = field.field.type;
  if (t === 'SIGNING_FIELD_TYPE_SIGNATURE' || t === 'signature')
    return 'signature';
  if (t === 'SIGNING_FIELD_TYPE_CHECKBOX' || t === 'checkbox')
    return 'checkbox';
  if (t === 'SIGNING_FIELD_TYPE_DATE' || t === 'date') return 'date';
  return 'input';
}
</script>

<template>
  <div class="min-h-screen bg-gray-50 dark:bg-gray-900">
    <!-- Header -->
    <div
      class="sticky top-0 z-40 border-b bg-white px-4 py-3 shadow-sm dark:bg-gray-800 sm:px-6"
    >
      <div class="mx-auto flex max-w-5xl items-center justify-between">
        <h1 class="text-lg font-semibold text-gray-800 dark:text-gray-100">
          {{ session?.requestName || 'Document Signing' }}
        </h1>
        <span
          v-if="session"
          class="text-sm text-gray-500 dark:text-gray-400"
        >
          Signing as:
          <strong class="text-gray-700 dark:text-gray-300">
            {{ session.recipientName }}
          </strong>
        </span>
      </div>
    </div>

    <!-- Loading -->
    <div
      v-if="loading"
      class="flex items-center justify-center py-20"
    >
      <Spin
        size="large"
        tip="Loading signing session..."
      />
    </div>

    <!-- Error -->
    <Result v-else-if="error" status="error" :title="error">
      <template #subTitle>
        <p class="text-gray-500">
          The signing link may be expired, invalid, or already used.
        </p>
      </template>
    </Result>

    <!-- Success -->
    <Result
      v-else-if="success"
      status="success"
      title="Document Signed Successfully"
      sub-title="Your signature has been recorded. You can close this page."
    />

    <!-- ========== OVERLAY MODE ========== -->
    <template v-else-if="session && layoutMode === 'overlay'">
      <div class="mx-auto max-w-4xl px-4 pb-[280px] pt-4">
        <!-- Message banner -->
        <div
          v-if="session.message"
          class="mb-4 rounded-lg bg-blue-50 p-3 text-sm text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
        >
          {{ session.message }}
        </div>

        <!-- PDF with overlays -->
        <PdfDocumentViewer
          :document-url="session.documentUrl"
          :fields="fields"
          :active-field-index="activeFieldIndex"
          :is-field-filled="isFieldFilled"
          @field-click="handleFieldClick"
        />
      </div>

      <!-- Sticky bottom panel -->
      <SigningPanel
        :active-field="activeField"
        :active-index="activeFieldIndex"
        :total-fields="totalFields"
        :filled-count="progress.filled"
        :can-submit="canSubmit"
        :is-last-field="isLastField"
        :submitting="submitting"
        @next="goToNext"
        @previous="goToPrevious"
        @submit="handleSubmit"
        @update:value="handleFieldValueUpdate"
        @update:signature="handleSignatureUpdate"
      />
    </template>

    <!-- ========== SIDEBAR FALLBACK MODE ========== -->
    <template v-else-if="session && layoutMode === 'sidebar'">
      <div class="mx-auto flex max-w-6xl gap-6 px-4 py-4 sm:px-6">
        <!-- Left: PDF -->
        <div class="min-w-0 flex-1">
          <div v-if="session.message" class="mb-4 rounded-lg bg-blue-50 p-3 text-sm text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
            {{ session.message }}
          </div>
          <PdfDocumentViewer
            :document-url="session.documentUrl"
            :fields="[]"
            :active-field-index="-1"
            :is-field-filled="isFieldFilled"
          />
        </div>

        <!-- Right: Field form -->
        <div class="w-80 flex-shrink-0 lg:w-96">
          <div class="sticky top-[60px] space-y-4">
            <Card title="Fill in the fields" size="small">
              <div class="space-y-4">
                <div
                  v-for="(flowField, idx) in fields"
                  :key="flowField.field.fieldId"
                >
                  <label class="mb-1 block text-sm font-medium">
                    {{ flowField.field.name }}
                    <span v-if="flowField.field.required" class="text-red-500"
                      >*</span
                    >
                  </label>

                  <!-- Signature -->
                  <SignatureCanvas
                    v-if="getFieldComponent(flowField) === 'signature'"
                    :model-value="flowField.signatureImage"
                    @update:model-value="setSignatureImage(idx, $event)"
                  />

                  <!-- Date -->
                  <DatePicker
                    v-else-if="getFieldComponent(flowField) === 'date'"
                    :value="flowField.value ? dayjs(flowField.value) : undefined"
                    style="width: 100%"
                    :placeholder="flowField.field.name"
                    @change="(_date: any, dateString: any) => setFieldValue(idx, Array.isArray(dateString) ? dateString[0] ?? '' : dateString ?? '')"
                  />

                  <!-- Checkbox -->
                  <Checkbox
                    v-else-if="getFieldComponent(flowField) === 'checkbox'"
                    :checked="flowField.value === 'true'"
                    @change="
                      (e: any) =>
                        setFieldValue(
                          idx,
                          e.target.checked ? 'true' : 'false',
                        )
                    "
                  >
                    {{ flowField.field.name }}
                  </Checkbox>

                  <!-- Text -->
                  <Input
                    v-else
                    :value="flowField.value"
                    :placeholder="flowField.field.name"
                    @update:value="setFieldValue(idx, $event)"
                  />
                </div>
              </div>
            </Card>

            <div class="flex justify-center">
              <Button
                type="primary"
                size="large"
                block
                :disabled="!canSubmit"
                :loading="submitting"
                @click="handleSubmit"
              >
                Sign Document
              </Button>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
