<script lang="ts" setup>
import { ref, computed } from 'vue';

import { useVbenDrawer } from 'shell/vben/common-ui';

import {
  Form,
  FormItem,
  Input,
  InputNumber,
  Button,
  notification,
  Textarea,
  Select,
} from 'ant-design-vue';

import { $t } from 'shell/locales';
import { usePaperlessCategoryStore } from '../../stores/paperless-category.state';

interface CategoryRow {
  id?: string;
  name?: string;
  description?: string;
  parentId?: string;
  sortOrder?: number;
}

const categoryStore = usePaperlessCategoryStore();

const data = ref<{
  mode: 'create' | 'edit' | 'view';
  parentId?: string;
  category?: CategoryRow;
}>();
const loading = ref(false);
const parentCategories = ref<Array<{ value: string; label: string }>>([]);

const formState = ref<{
  name: string;
  description: string;
  parentId?: string;
  sortOrder: number;
}>({
  name: '',
  description: '',
  parentId: undefined,
  sortOrder: 0,
});

const title = computed(() => {
  switch (data.value?.mode) {
    case 'create':
      return $t('paperless.page.category.create');
    case 'edit':
      return $t('paperless.page.category.edit');
    default:
      return $t('ui.button.view');
  }
});

const isCreateMode = computed(() => data.value?.mode === 'create');
const isEditMode = computed(() => data.value?.mode === 'edit');
const isViewMode = computed(() => data.value?.mode === 'view');

async function loadParentCategories() {
  try {
    const resp = await categoryStore.listCategories({ page: 1, pageSize: 1000 });
    parentCategories.value = (resp.categories ?? resp.items ?? [])
      .filter((cat: { id?: string }) => cat.id !== data.value?.category?.id)
      .map((cat: { id?: string; name?: string }) => ({
        value: cat.id ?? '',
        label: cat.name ?? '',
      }));
  } catch (e) {
    console.error('Failed to load parent categories:', e);
  }
}

async function handleSubmit() {
  if (!formState.value.name.trim()) {
    notification.error({
      message: $t('ui.formRules.required'),
    });
    return;
  }

  loading.value = true;
  try {
    if (isCreateMode.value) {
      await categoryStore.createCategory({
        name: formState.value.name,
        description: formState.value.description || undefined,
        parentId: formState.value.parentId,
      });
      notification.success({
        message: $t('paperless.page.category.createSuccess'),
      });
    } else if (isEditMode.value && data.value?.category?.id) {
      await categoryStore.updateCategory(data.value.category.id, {
        name: formState.value.name,
        description: formState.value.description || undefined,
      });
      notification.success({
        message: $t('paperless.page.category.updateSuccess'),
      });
    }
    drawerApi.close();
  } catch (e: any) {
    console.error('Failed to save category:', e);
    notification.error({
      message: isCreateMode.value
        ? $t('ui.notification.create_failed')
        : $t('ui.notification.update_failed'),
      description: e.message,
    });
  } finally {
    loading.value = false;
  }
}

function resetForm() {
  formState.value = {
    name: '',
    description: '',
    parentId: undefined,
    sortOrder: 0,
  };
}

const [Drawer, drawerApi] = useVbenDrawer({
  onCancel() {
    drawerApi.close();
  },

  async onOpenChange(isOpen) {
    if (isOpen) {
      data.value = drawerApi.getData() as {
        mode: 'create' | 'edit' | 'view';
        parentId?: string;
        category?: CategoryRow;
      };

      await loadParentCategories();

      if (data.value?.mode === 'create') {
        resetForm();
        if (data.value.parentId) {
          formState.value.parentId = data.value.parentId;
        }
      } else if (data.value?.category) {
        formState.value = {
          name: data.value.category.name ?? '',
          description: data.value.category.description ?? '',
          parentId: data.value.category.parentId,
          sortOrder: data.value.category.sortOrder ?? 0,
        };
      }
    }
  },
});
</script>

<template>
  <Drawer :title="title" :footer="false" class="w-[500px]">
    <Form layout="vertical" :model="formState" @finish="handleSubmit">
      <FormItem
        :label="$t('paperless.page.category.name')"
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

      <FormItem :label="$t('paperless.page.category.parent')" name="parentId">
        <Select
          v-model:value="formState.parentId"
          :options="parentCategories"
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

      <FormItem :label="$t('paperless.page.category.description')" name="description">
        <Textarea
          v-model:value="formState.description"
          :rows="4"
          :maxlength="1024"
          :placeholder="$t('ui.placeholder.input')"
          :disabled="isViewMode"
        />
      </FormItem>

      <FormItem :label="$t('paperless.page.category.sortOrder')" name="sortOrder">
        <InputNumber
          v-model:value="formState.sortOrder"
          :min="0"
          :placeholder="$t('ui.placeholder.input')"
          :disabled="isViewMode"
          style="width: 100%"
        />
      </FormItem>

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
