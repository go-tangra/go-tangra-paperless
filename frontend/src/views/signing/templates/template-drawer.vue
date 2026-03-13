<script lang="ts" setup>
import { ref, computed } from 'vue';
import { useRouter } from 'vue-router';

import { useVbenDrawer } from 'shell/vben/common-ui';
import { LucideUpload, LucideFile, LucideX } from 'shell/vben/icons';

import {
  Form,
  FormItem,
  Input,
  Button,
  notification,
  Textarea,
} from 'ant-design-vue';
import type { UploadFile } from 'ant-design-vue';

import { $t } from 'shell/locales';
import { usePaperlessSigningTemplateStore } from '../../../stores/paperless-signing-template.state';
import type { SigningTemplate } from '../../../stores/paperless-signing-template.state';

const templateStore = usePaperlessSigningTemplateStore();
const router = useRouter();

const data = ref<{
  mode: 'create' | 'edit' | 'view';
  row?: SigningTemplate;
}>();
const loading = ref(false);
const fileList = ref<UploadFile[]>([]);
const fileInputRef = ref<HTMLInputElement | null>(null);
const isDragOver = ref(false);

const formState = ref<{
  name: string;
  description: string;
  revocationStampText: string;
}>({
  name: '',
  description: '',
  revocationStampText: '',
});

const title = computed(() => {
  switch (data.value?.mode) {
    case 'create': return $t('paperless.page.signingTemplate.create');
    case 'edit': return $t('paperless.page.signingTemplate.edit');
    default: return $t('ui.button.view');
  }
});

const isCreateMode = computed(() => data.value?.mode === 'create');
const isViewMode = computed(() => data.value?.mode === 'view');

async function handleSubmit() {
  if (!formState.value.name.trim()) {
    notification.error({
      message: $t('ui.formRules.required'),
      description: $t('paperless.page.signingTemplate.name'),
    });
    return;
  }

  if (isCreateMode.value && fileList.value.length === 0) {
    notification.error({
      message: $t('ui.formRules.required'),
      description: $t('paperless.page.signingTemplate.selectFile'),
    });
    return;
  }

  loading.value = true;
  try {
    if (isCreateMode.value) {
      const uploadFile = fileList.value[0];
      const file = uploadFile?.originFileObj ?? (uploadFile as unknown as File);

      if (!file || !(file instanceof File)) {
        notification.error({ message: $t('paperless.page.signingTemplate.selectFile') });
        loading.value = false;
        return;
      }

      const resp = await templateStore.createSigningTemplate(
        {
          name: formState.value.name,
          description: formState.value.description || undefined,
          revocationStampText: formState.value.revocationStampText || undefined,
        },
        file,
      );
      notification.success({ message: $t('paperless.page.signingTemplate.createSuccess') });
      if (resp.template?.id) {
        drawerApi.close();
        router.push(`/paperless/signing/templates/${resp.template.id}/builder`);
        return;
      }
    } else if (data.value?.row?.id) {
      await templateStore.updateSigningTemplate(data.value.row.id, {
        name: formState.value.name,
        description: formState.value.description || undefined,
        revocationStampText: formState.value.revocationStampText || undefined,
      });
      notification.success({ message: $t('paperless.page.signingTemplate.updateSuccess') });
    }
    drawerApi.close();
  } catch (e: any) {
    const errorMessage = e?.message || String(e);
    notification.error({
      message: isCreateMode.value
        ? $t('ui.notification.create_failed')
        : $t('ui.notification.update_failed'),
      description: errorMessage,
    });
  } finally {
    loading.value = false;
  }
}

function resetForm() {
  formState.value = { name: '', description: '', revocationStampText: '' };
  fileList.value = [];
}

function triggerFileInput() {
  setTimeout(() => {
    fileInputRef.value?.click();
  }, 0);
}

function handleFileInputChange(e: Event) {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  if (file) {
    addFile(file);
  }
  input.value = '';
}

function handleDrop(e: DragEvent) {
  e.preventDefault();
  isDragOver.value = false;
  const file = e.dataTransfer?.files[0];
  if (file) {
    addFile(file);
  }
}

function handleDragOver(e: DragEvent) {
  e.preventDefault();
  isDragOver.value = true;
}

function handleDragLeave() {
  isDragOver.value = false;
}

function addFile(file: File) {
  if (!file.name.toLowerCase().endsWith('.pdf')) {
    notification.error({ message: 'Only PDF files are supported' });
    return;
  }

  fileList.value = [
    {
      uid: `-${Date.now()}`,
      name: file.name,
      size: file.size,
      type: file.type,
      status: 'done',
      originFileObj: file,
    } as UploadFile,
  ];

  if (!formState.value.name) {
    const nameWithoutExt = file.name.replace(/\.[^/.]+$/, '');
    formState.value.name = nameWithoutExt;
  }
}

function formatFileSize(bytes: number | undefined): string {
  if (!bytes) return '-';
  const size = typeof bytes === 'string' ? parseInt(bytes, 10) : bytes;
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

const [Drawer, drawerApi] = useVbenDrawer({
  onCancel() {
    drawerApi.close();
  },
  async onOpenChange(isOpen) {
    if (isOpen) {
      data.value = drawerApi.getData() as {
        mode: 'create' | 'edit' | 'view';
        row?: SigningTemplate;
      };

      if (data.value?.mode === 'create') {
        resetForm();
      } else if (data.value?.row) {
        formState.value = {
          name: data.value.row.name ?? '',
          description: data.value.row.description ?? '',
          revocationStampText: data.value.row.revocationStampText ?? '',
        };
        fileList.value = [];
      }
    }
  },
});
</script>

<template>
  <Drawer :title="title" :footer="false" class="w-[650px]">
    <Form layout="vertical" :model="formState" @finish="handleSubmit">
      <FormItem
        :label="$t('paperless.page.signingTemplate.name')"
        name="name"
        :rules="[{ required: true, message: $t('ui.formRules.required') }]"
      >
        <Input
          v-model:value="formState.name"
          :placeholder="$t('ui.placeholder.input')"
          :maxlength="255"
          :disabled="isViewMode"
        />
      </FormItem>

      <FormItem :label="$t('paperless.page.signingTemplate.description')" name="description">
        <Textarea
          v-model:value="formState.description"
          :rows="3"
          :maxlength="1024"
          :placeholder="$t('ui.placeholder.input')"
          :disabled="isViewMode"
        />
      </FormItem>

      <FormItem :label="$t('paperless.page.signingTemplate.revocationStampText')" name="revocationStampText">
        <Input
          v-model:value="formState.revocationStampText"
          :placeholder="$t('paperless.page.signingTemplate.revocationStampTextPlaceholder')"
          :maxlength="255"
          :disabled="isViewMode"
        />
      </FormItem>

      <!-- File Upload for Create Mode -->
      <FormItem
        v-if="isCreateMode"
        :label="$t('paperless.page.signingTemplate.upload')"
        :rules="[{ required: true, message: $t('ui.formRules.required') }]"
      >
        <Teleport to="body">
          <input
            ref="fileInputRef"
            type="file"
            accept=".pdf"
            style="position: fixed; top: -9999px; left: -9999px; opacity: 0"
            @change="handleFileInputChange"
          />
        </Teleport>
        <div
          v-if="fileList.length === 0"
          class="border-2 border-dashed rounded-lg p-8 text-center transition-colors cursor-pointer"
          :class="isDragOver ? 'border-primary bg-primary/5' : 'border-gray-300 hover:border-primary'"
          @click.stop="triggerFileInput"
          @drop.prevent.stop="handleDrop"
          @dragover.prevent.stop="handleDragOver"
          @dragleave="handleDragLeave"
        >
          <div class="flex flex-col items-center gap-4 pointer-events-none">
            <LucideUpload class="size-12 text-gray-400" />
            <div>
              <p class="text-base font-medium">
                {{ $t('paperless.page.signingTemplate.dropFileHere') }}
              </p>
              <p class="text-sm text-gray-500">PDF only</p>
            </div>
          </div>
        </div>
        <div v-else class="flex items-center gap-2 p-3 bg-accent rounded-lg">
          <LucideFile class="size-5 text-blue-500" />
          <span class="font-medium">{{ fileList[0]?.name }}</span>
          <span class="text-sm text-gray-500">
            ({{ formatFileSize(fileList[0]?.size) }})
          </span>
          <Button
            type="text"
            size="small"
            class="ml-auto text-gray-400 hover:text-red-500"
            @click="fileList = []"
          >
            <LucideX class="size-4" />
          </Button>
        </div>
      </FormItem>

      <!-- File Info for Edit/View Mode -->
      <div v-if="!isCreateMode && data?.row" class="mb-4 rounded bg-accent p-4">
        <h4 class="mb-2 font-medium">{{ $t('paperless.page.document.fileInfo') }}</h4>
        <div class="space-y-1 text-sm">
          <p>
            <span class="text-gray-500">{{ $t('paperless.page.signingTemplate.fileName') }}:</span>
            {{ data.row.fileName || '-' }}
          </p>
          <p>
            <span class="text-gray-500">{{ $t('paperless.page.signingTemplate.fileSize') }}:</span>
            {{ formatFileSize(data.row.fileSize) }}
          </p>
        </div>
      </div>

      <!-- Edit Fields Button for View Mode -->
      <div v-if="data?.row?.id" class="mb-4">
        <Button type="primary" ghost @click="router.push(`/paperless/signing/templates/${data.row.id}/builder`)">
          {{ $t('paperless.page.builder.editFields') }}
        </Button>
      </div>

      <FormItem v-if="!isViewMode">
        <div class="flex justify-end gap-2">
          <Button @click="drawerApi.close()">
            {{ $t('ui.button.cancel') }}
          </Button>
          <Button type="primary" html-type="submit" :loading="loading">
            {{ $t('ui.button.save') }}
          </Button>
        </div>
      </FormItem>
      <FormItem v-else>
        <div class="flex justify-end">
          <Button @click="drawerApi.close()">
            {{ $t('ui.button.close') }}
          </Button>
        </div>
      </FormItem>
    </Form>
  </Drawer>
</template>
