<script lang="ts" setup>
import { ref, computed } from 'vue';

import { useVbenDrawer } from 'shell/vben/common-ui';
import { LucideUpload, LucideFile, LucideX } from 'shell/vben/icons';

import {
  Form,
  FormItem,
  Input,
  Button,
  notification,
  Textarea,
  Select,
} from 'ant-design-vue';
import type { UploadFile } from 'ant-design-vue';

import { $t } from 'shell/locales';
import { usePaperlessDocumentStore } from '../../stores/paperless-document.state';
import { usePaperlessCategoryStore } from '../../stores/paperless-category.state';

interface DocumentRow {
  id?: string;
  name?: string;
  description?: string;
  categoryId?: string;
  fileName?: string;
  mimeType?: string;
  fileSize?: number;
  status?: string;
}

const documentStore = usePaperlessDocumentStore();
const categoryStore = usePaperlessCategoryStore();

const data = ref<{
  mode: 'create' | 'edit' | 'view';
  categoryId?: string;
  row?: DocumentRow;
}>();
const loading = ref(false);
const categories = ref<Array<{ value: string; label: string }>>([]);
const fileList = ref<UploadFile[]>([]);
const fileInputRef = ref<HTMLInputElement | null>(null);
const isDragOver = ref(false);

const formState = ref<{
  name: string;
  description: string;
  categoryId?: string;
}>({
  name: '',
  description: '',
  categoryId: undefined,
});

const title = computed(() => {
  switch (data.value?.mode) {
    case 'create':
      return $t('paperless.page.document.create');
    case 'edit':
      return $t('paperless.page.document.edit');
    default:
      return $t('ui.button.view');
  }
});

const isCreateMode = computed(() => data.value?.mode === 'create');
const isEditMode = computed(() => data.value?.mode === 'edit');
const isViewMode = computed(() => data.value?.mode === 'view');

async function loadCategories() {
  try {
    const resp = await categoryStore.listCategories({ page: 1, pageSize: 1000 });
    categories.value = (resp.categories ?? resp.items ?? []).map((cat: { id?: string; name?: string }) => ({
      value: cat.id ?? '',
      label: cat.name ?? '',
    }));
  } catch (e) {
    console.error('Failed to load categories:', e);
  }
}

async function handleSubmit() {
  if (!formState.value.name.trim()) {
    notification.error({
      message: $t('ui.formRules.required'),
      description: $t('paperless.page.document.name'),
    });
    return;
  }

  if (isCreateMode.value && fileList.value.length === 0) {
    notification.error({
      message: $t('ui.formRules.required'),
      description: $t('paperless.page.document.uploadFile'),
    });
    return;
  }

  loading.value = true;
  try {
    if (isCreateMode.value) {
      // Get the file - try originFileObj first, then the file itself
      const uploadFile = fileList.value[0];
      const file = uploadFile?.originFileObj ?? (uploadFile as unknown as File);

      if (!file || !(file instanceof File)) {
        console.error('Invalid file object:', uploadFile);
        notification.error({
          message: $t('paperless.page.document.selectFile'),
        });
        loading.value = false;
        return;
      }

      await documentStore.uploadDocument(
        {
          name: formState.value.name,
          description: formState.value.description || undefined,
          categoryId: formState.value.categoryId,
        },
        file,
      );
      notification.success({
        message: $t('paperless.page.document.createSuccess'),
      });
    } else if (isEditMode.value && data.value?.row?.id) {
      await documentStore.updateDocument(data.value.row.id, {
        name: formState.value.name,
        description: formState.value.description || undefined,
      });
      notification.success({
        message: $t('paperless.page.document.updateSuccess'),
      });
    }
    drawerApi.close();
  } catch (e: any) {
    console.error('Failed to save document:', e);
    const errorMessage = e?.message || e?.reason || String(e);
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
  formState.value = {
    name: '',
    description: '',
    categoryId: undefined,
  };
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

function formatFileSize(bytes: string | number | undefined): string {
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
        categoryId?: string;
        row?: DocumentRow;
      };

      await loadCategories();

      if (data.value?.mode === 'create') {
        resetForm();
        if (data.value.categoryId) {
          formState.value.categoryId = data.value.categoryId;
        }
      } else if (data.value?.row) {
        formState.value = {
          name: data.value.row.name ?? '',
          description: data.value.row.description ?? '',
          categoryId: data.value.row.categoryId,
        };
        fileList.value = [];
      }
    }
  },
});
</script>

<template>
  <Drawer :title="title" :footer="false" class="w-[600px]">
    <Form layout="vertical" :model="formState" @finish="handleSubmit">
      <FormItem
        :label="$t('paperless.page.document.name')"
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

      <FormItem :label="$t('paperless.page.document.category')" name="categoryId">
        <Select
          v-model:value="formState.categoryId"
          :options="categories"
          :placeholder="$t('ui.placeholder.select')"
          allow-clear
          show-search
          :filter-option="
            (input: string, option: any) =>
              option.label.toLowerCase().includes(input.toLowerCase())
          "
          :disabled="isViewMode"
        />
      </FormItem>

      <FormItem :label="$t('paperless.page.document.description')" name="description">
        <Textarea
          v-model:value="formState.description"
          :rows="4"
          :maxlength="1024"
          :placeholder="$t('ui.placeholder.input')"
          :disabled="isViewMode"
        />
      </FormItem>

      <!-- File Upload for Create Mode -->
      <FormItem
        v-if="isCreateMode"
        :label="$t('paperless.page.document.uploadFile')"
        :rules="[{ required: true, message: $t('ui.formRules.required') }]"
      >
        <Teleport to="body">
          <input
            ref="fileInputRef"
            type="file"
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
                {{ $t('paperless.page.document.dropFileHere') }}
              </p>
            </div>
          </div>
        </div>
        <div v-else class="flex items-center gap-2 p-3 bg-gray-50 rounded-lg dark:bg-gray-800">
          <LucideFile class="size-5" style="color: #3b82f6" />
          <span class="font-medium">{{ fileList[0]?.name }}</span>
          <span class="text-sm text-gray-500">
            ({{ formatFileSize(fileList[0]?.size) }})
          </span>
          <Button
            type="text"
            size="small"
            class="ml-auto file-remove-btn"
            @click="fileList = []"
          >
            <LucideX class="size-4" />
          </Button>
        </div>
      </FormItem>

      <!-- File Info for Edit/View Mode -->
      <div v-if="!isCreateMode && data?.row" class="mb-4 rounded bg-gray-50 p-4 dark:bg-gray-800">
        <h4 class="mb-2 font-medium">{{ $t('paperless.page.document.fileInfo') }}</h4>
        <div class="space-y-1 text-sm">
          <p>
            <span class="text-gray-500">{{ $t('paperless.page.document.fileName') }}:</span>
            {{ data.row.fileName || '-' }}
          </p>
          <p>
            <span class="text-gray-500">{{ $t('paperless.page.document.fileSize') }}:</span>
            {{ formatFileSize(data.row.fileSize) }}
          </p>
          <p>
            <span class="text-gray-500">{{ $t('paperless.page.document.mimeType') }}:</span>
            {{ data.row.mimeType || '-' }}
          </p>
        </div>
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

<style scoped>
.file-remove-btn {
  color: #9ca3af;
}
.file-remove-btn:hover {
  color: #ef4444;
}
</style>
