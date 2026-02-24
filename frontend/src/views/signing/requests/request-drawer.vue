<script lang="ts" setup>
import { ref, computed, h } from 'vue';

import { useVbenDrawer } from 'shell/vben/common-ui';
import { LucidePlus, LucideTrash, LucideMail } from 'shell/vben/icons';

import {
  Form,
  FormItem,
  Input,
  Button,
  notification,
  Textarea,
  Select,
  SelectOption,
  DatePicker,
  Table,
  Tag,
  InputNumber,
  Space,
} from 'ant-design-vue';

import { $t } from 'shell/locales';
import { usePaperlessSigningRequestStore } from '../../../stores/paperless-signing-request.state';
import { usePaperlessSigningTemplateStore } from '../../../stores/paperless-signing-template.state';
import type { SigningRequest, SigningRecipient } from '../../../stores/paperless-signing-request.state';
import type { SigningTemplate } from '../../../stores/paperless-signing-template.state';

const requestStore = usePaperlessSigningRequestStore();
const templateStore = usePaperlessSigningTemplateStore();

interface RecipientInput {
  email: string;
  name: string;
  signingOrder: number;
}

const data = ref<{
  mode: 'create' | 'view';
  row?: SigningRequest;
}>();
const loading = ref(false);
const templates = ref<Array<{ value: string; label: string }>>([]);
const selectedTemplate = ref<SigningTemplate | null>(null);
const recipients = ref<RecipientInput[]>([]);
const prefillValues = ref<Record<string, string>>({});

const prefillFieldsByStage = computed(() => {
  const fields = selectedTemplate.value?.fields ?? [];
  const stages = new Map<number, typeof fields>();
  for (const f of fields) {
    if (f.prefillStage && f.prefillStage > 0) {
      const list = stages.get(f.prefillStage) ?? [];
      list.push(f);
      stages.set(f.prefillStage, list);
    }
  }
  return [...stages.entries()].sort((a, b) => a[0] - b[0]);
});

const formState = ref<{
  name: string;
  templateId: string | undefined;
  message: string;
  expiresAt: string | undefined;
}>({
  name: '',
  templateId: undefined,
  message: '',
  expiresAt: undefined,
});

const title = computed(() => {
  return data.value?.mode === 'create'
    ? $t('paperless.page.signingRequest.create')
    : $t('paperless.page.signingRequest.view');
});

const isCreateMode = computed(() => data.value?.mode === 'create');

function getRecipientStatusColor(status: string | undefined): string {
  switch (status) {
    case 'SIGNING_RECIPIENT_STATUS_WAITING': return 'default';
    case 'SIGNING_RECIPIENT_STATUS_PENDING': return 'processing';
    case 'SIGNING_RECIPIENT_STATUS_COMPLETED': return 'success';
    case 'SIGNING_RECIPIENT_STATUS_DECLINED': return 'error';
    default: return 'default';
  }
}

function getRecipientStatusLabel(status: string | undefined): string {
  switch (status) {
    case 'SIGNING_RECIPIENT_STATUS_WAITING': return $t('paperless.page.signingRequest.recipientWaiting');
    case 'SIGNING_RECIPIENT_STATUS_PENDING': return $t('paperless.page.signingRequest.recipientPending');
    case 'SIGNING_RECIPIENT_STATUS_COMPLETED': return $t('paperless.page.signingRequest.recipientCompleted');
    case 'SIGNING_RECIPIENT_STATUS_DECLINED': return $t('paperless.page.signingRequest.recipientDeclined');
    default: return '-';
  }
}

const recipientColumns = [
  { title: '#', dataIndex: 'signingOrder', key: 'order', width: 50 },
  { title: $t('paperless.page.signingRequest.recipientName'), dataIndex: 'name', key: 'name' },
  { title: $t('paperless.page.signingRequest.recipientEmail'), dataIndex: 'email', key: 'email' },
  ...(data.value?.mode === 'view'
    ? [
        { title: $t('paperless.page.signingRequest.status'), dataIndex: 'status', key: 'status', width: 120 },
      ]
    : [
        { title: '', key: 'actions', width: 60 },
      ]),
];

const viewRecipientColumns = [
  { title: '#', dataIndex: 'signingOrder', key: 'order', width: 50 },
  { title: $t('paperless.page.signingRequest.recipientName'), dataIndex: 'name', key: 'name' },
  { title: $t('paperless.page.signingRequest.recipientEmail'), dataIndex: 'email', key: 'email' },
  { title: $t('paperless.page.signingRequest.status'), dataIndex: 'status', key: 'status', width: 120 },
  { title: '', key: 'actions', width: 80 },
];

async function loadTemplates() {
  try {
    const resp = await templateStore.listSigningTemplates({ page: 1, pageSize: 1000 });
    templates.value = (resp.templates ?? []).map((t) => ({
      value: t.id ?? '',
      label: t.name ?? '',
    }));
  } catch (e) {
    console.error('Failed to load templates:', e);
  }
}

async function handleTemplateChange(templateId: string) {
  try {
    const resp = await templateStore.getSigningTemplate(templateId);
    selectedTemplate.value = resp.template ?? null;
    prefillValues.value = {};
  } catch {
    selectedTemplate.value = null;
    prefillValues.value = {};
  }
}

function addRecipient() {
  const nextOrder = recipients.value.length + 1;
  recipients.value.push({
    email: '',
    name: '',
    signingOrder: nextOrder,
  });
}

function removeRecipient(index: number) {
  recipients.value.splice(index, 1);
  // Reorder
  recipients.value.forEach((r, i) => { r.signingOrder = i + 1; });
}

async function handleSubmit() {
  if (!formState.value.name.trim()) {
    notification.error({
      message: $t('ui.formRules.required'),
      description: $t('paperless.page.signingRequest.name'),
    });
    return;
  }

  if (!formState.value.templateId) {
    notification.error({
      message: $t('ui.formRules.required'),
      description: $t('paperless.page.signingRequest.noTemplate'),
    });
    return;
  }

  if (!selectedTemplate.value?.fields?.length) {
    notification.error({
      message: $t('paperless.page.signingTemplate.noFields'),
    });
    return;
  }

  if (recipients.value.length === 0) {
    notification.error({
      message: $t('ui.formRules.required'),
      description: $t('paperless.page.signingRequest.addRecipient'),
    });
    return;
  }

  // Validate recipients
  for (const r of recipients.value) {
    if (!r.email.trim() || !r.name.trim()) {
      notification.error({
        message: $t('ui.formRules.required'),
        description: `${$t('paperless.page.signingRequest.recipientEmail')} / ${$t('paperless.page.signingRequest.recipientName')}`,
      });
      return;
    }
  }

  loading.value = true;
  try {
    // Build field values from prefill inputs
    const fieldValues: Array<{ fieldId: string; value: string }> = [];
    for (const [fieldId, value] of Object.entries(prefillValues.value)) {
      if (value.trim()) {
        fieldValues.push({ fieldId, value });
      }
    }

    await requestStore.createSigningRequest({
      templateId: formState.value.templateId,
      name: formState.value.name,
      recipients: recipients.value.map((r) => ({
        email: r.email,
        name: r.name,
        signingOrder: r.signingOrder,
      })),
      fieldValues: fieldValues.length > 0 ? fieldValues : undefined,
      message: formState.value.message || undefined,
      expiresAt: formState.value.expiresAt || undefined,
    });
    notification.success({ message: $t('paperless.page.signingRequest.createSuccess') });
    drawerApi.close();
  } catch (e: any) {
    const errorMessage = e?.message || String(e);
    notification.error({
      message: $t('ui.notification.create_failed'),
      description: errorMessage,
    });
  } finally {
    loading.value = false;
  }
}

async function handleResend(recipient: SigningRecipient) {
  if (!data.value?.row?.id || !recipient.id) return;
  try {
    await requestStore.resendSigningEmail(data.value.row.id, recipient.id);
    notification.success({ message: $t('paperless.page.signingRequest.resendSuccess') });
  } catch {
    notification.error({ message: $t('ui.notification.operation_failed') });
  }
}

function resetForm() {
  formState.value = {
    name: '',
    templateId: undefined,
    message: '',
    expiresAt: undefined,
  };
  recipients.value = [];
  selectedTemplate.value = null;
  prefillValues.value = {};
}

const [Drawer, drawerApi] = useVbenDrawer({
  onCancel() {
    drawerApi.close();
  },
  async onOpenChange(isOpen) {
    if (isOpen) {
      data.value = drawerApi.getData() as {
        mode: 'create' | 'view';
        row?: SigningRequest;
      };

      if (data.value?.mode === 'create') {
        resetForm();
        await loadTemplates();
      }
    }
  },
});
</script>

<template>
  <Drawer :title="title" :footer="false" class="w-[700px]">
    <!-- Create Mode -->
    <Form v-if="isCreateMode" layout="vertical" :model="formState" @finish="handleSubmit">
      <FormItem
        :label="$t('paperless.page.signingRequest.name')"
        name="name"
        :rules="[{ required: true, message: $t('ui.formRules.required') }]"
      >
        <Input
          v-model:value="formState.name"
          :placeholder="$t('ui.placeholder.input')"
          :maxlength="255"
        />
      </FormItem>

      <FormItem
        :label="$t('paperless.page.signingRequest.template')"
        name="templateId"
        :rules="[{ required: true, message: $t('ui.formRules.required') }]"
      >
        <Select
          v-model:value="formState.templateId"
          :options="templates"
          :placeholder="$t('ui.placeholder.select')"
          show-search
          :filter-option="(input: string, option: any) => option.label.toLowerCase().includes(input.toLowerCase())"
          @change="handleTemplateChange"
        />
      </FormItem>

      <!-- Template fields preview -->
      <div v-if="selectedTemplate?.fields?.length" class="mb-4 rounded bg-gray-50 p-3 dark:bg-gray-800">
        <p class="mb-1 text-sm font-medium text-gray-600">
          {{ $t('paperless.page.signingTemplate.fields') }}:
        </p>
        <div class="flex flex-wrap gap-1">
          <Tag v-for="field in selectedTemplate.fields" :key="field.id" :color="field.prefillStage ? 'orange' : field.recipientIndex ? 'green' : 'blue'">
            {{ field.name }}
            <span class="text-xs opacity-75">p{{ field.pageNumber }}</span>
            <span v-if="field.prefillStage" class="text-xs opacity-75">(prefill #{{ field.prefillStage }})</span>
            <span v-else-if="field.recipientIndex" class="text-xs opacity-75">(recipient #{{ field.recipientIndex }})</span>
          </Tag>
        </div>
      </div>
      <div v-else-if="selectedTemplate && (!selectedTemplate.fields || selectedTemplate.fields.length === 0)" class="mb-4 rounded border border-orange-300 bg-orange-50 p-3 text-sm text-orange-700 dark:border-orange-600 dark:bg-orange-900/20 dark:text-orange-400">
        {{ $t('paperless.page.signingTemplate.noFields') }}
      </div>

      <!-- Prefill field values (grouped by stage) -->
      <div v-if="prefillFieldsByStage.length > 0" class="mb-4">
        <p class="mb-2 text-sm font-medium">{{ $t('paperless.page.signingRequest.prefillValues') }}</p>
        <div v-for="[stage, stageFields] in prefillFieldsByStage" :key="stage" class="mb-3 rounded border p-3">
          <p class="mb-2 text-xs font-medium text-gray-500">
            <template v-if="stage === 1">{{ $t('paperless.page.signingRequest.prefillAtCreation') }}</template>
            <template v-else>{{ $t('paperless.page.signingRequest.prefillBeforeRecipient', { n: stage }) }}</template>
          </p>
          <div v-for="field in stageFields" :key="field.id" class="mb-2">
            <label class="mb-1 block text-xs text-gray-600">{{ field.name }}</label>
            <Input
              :value="prefillValues[field.id ?? '']"
              :placeholder="field.name"
              size="small"
              @update:value="(val: string) => { prefillValues[field.id ?? ''] = val; }"
            />
          </div>
        </div>
      </div>

      <FormItem :label="$t('paperless.page.signingRequest.message')" name="message">
        <Textarea
          v-model:value="formState.message"
          :rows="2"
          :maxlength="1024"
          :placeholder="$t('ui.placeholder.input')"
        />
      </FormItem>

      <FormItem :label="$t('paperless.page.signingRequest.expiresAt')" name="expiresAt">
        <DatePicker
          v-model:value="formState.expiresAt"
          show-time
          :placeholder="$t('ui.placeholder.select')"
          style="width: 100%"
        />
      </FormItem>

      <!-- Recipients -->
      <div class="mb-4">
        <div class="mb-2 flex items-center justify-between">
          <span class="font-medium">{{ $t('paperless.page.signingRequest.recipients') }}</span>
          <Button type="dashed" size="small" @click="addRecipient">
            <template #icon><component :is="LucidePlus" class="size-3" /></template>
            {{ $t('paperless.page.signingRequest.addRecipient') }}
          </Button>
        </div>

        <div v-for="(recipient, index) in recipients" :key="index" class="mb-2 flex items-center gap-2 rounded border p-3">
          <InputNumber
            v-model:value="recipient.signingOrder"
            :min="1"
            :max="recipients.length"
            size="small"
            style="width: 60px"
          />
          <Input
            v-model:value="recipient.name"
            :placeholder="$t('paperless.page.signingRequest.recipientName')"
            size="small"
            style="flex: 1"
          />
          <Input
            v-model:value="recipient.email"
            :placeholder="$t('paperless.page.signingRequest.recipientEmail')"
            size="small"
            type="email"
            style="flex: 1"
          />
          <Button
            type="text"
            size="small"
            danger
            :icon="h(LucideTrash)"
            @click="removeRecipient(index)"
          />
        </div>

        <div v-if="recipients.length === 0" class="rounded border border-dashed p-4 text-center text-sm text-gray-400">
          {{ $t('paperless.page.signingRequest.addRecipient') }}
        </div>
      </div>

      <FormItem>
        <div class="flex justify-end gap-2">
          <Button @click="drawerApi.close()">
            {{ $t('ui.button.cancel') }}
          </Button>
          <Button type="primary" html-type="submit" :loading="loading">
            {{ $t('ui.button.save') }}
          </Button>
        </div>
      </FormItem>
    </Form>

    <!-- View Mode -->
    <div v-else-if="data?.row">
      <div class="space-y-4">
        <div class="rounded bg-gray-50 p-4 dark:bg-gray-800">
          <div class="grid grid-cols-2 gap-3 text-sm">
            <div>
              <span class="text-gray-500">{{ $t('paperless.page.signingRequest.name') }}:</span>
              <p class="font-medium">{{ data.row.name }}</p>
            </div>
            <div>
              <span class="text-gray-500">{{ $t('paperless.page.signingRequest.status') }}:</span>
              <p>
                <Tag :color="data.row.status === 'SIGNING_REQUEST_STATUS_COMPLETED' ? 'success' : data.row.status === 'SIGNING_REQUEST_STATUS_PENDING' ? 'processing' : 'default'">
                  {{ data.row.status?.replace('SIGNING_REQUEST_STATUS_', '') }}
                </Tag>
              </p>
            </div>
            <div>
              <span class="text-gray-500">{{ $t('paperless.page.signingRequest.template') }}:</span>
              <p>{{ data.row.templateName || '-' }}</p>
            </div>
            <div>
              <span class="text-gray-500">{{ $t('paperless.page.signingRequest.expiresAt') }}:</span>
              <p>{{ data.row.expiresAt || '-' }}</p>
            </div>
          </div>
          <div v-if="data.row.message" class="mt-3">
            <span class="text-sm text-gray-500">{{ $t('paperless.page.signingRequest.message') }}:</span>
            <p class="text-sm">{{ data.row.message }}</p>
          </div>
        </div>

        <!-- Recipients Table -->
        <div>
          <h4 class="mb-2 font-medium">{{ $t('paperless.page.signingRequest.recipients') }}</h4>
          <Table
            :columns="viewRecipientColumns"
            :data-source="data.row.recipients ?? []"
            :pagination="false"
            size="small"
            row-key="id"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'status'">
                <Tag :color="getRecipientStatusColor(record.status)">
                  {{ getRecipientStatusLabel(record.status) }}
                </Tag>
              </template>
              <template v-if="column.key === 'actions'">
                <Button
                  v-if="record.status === 'SIGNING_RECIPIENT_STATUS_PENDING'"
                  type="link"
                  size="small"
                  :icon="h(LucideMail)"
                  :title="$t('paperless.page.signingRequest.resend')"
                  @click="handleResend(record)"
                />
              </template>
            </template>
          </Table>
        </div>
      </div>

      <div class="mt-4 flex justify-end">
        <Button @click="drawerApi.close()">
          {{ $t('ui.button.close') }}
        </Button>
      </div>
    </div>
  </Drawer>
</template>
