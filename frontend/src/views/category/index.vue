<script lang="ts" setup>
import type { VxeGridProps } from 'shell/adapter/vxe-table';

import { h, ref, onMounted } from 'vue';

import { Page, useVbenDrawer, type VbenFormProps } from 'shell/vben/common-ui';
import {
  LucideEye,
  LucideTrash,
  LucidePencil,
  LucideFolderPlus,
} from 'shell/vben/icons';

import { notification, Space, Button, Modal } from 'ant-design-vue';

import { useVbenVxeGrid } from 'shell/adapter/vxe-table';
import { $t } from 'shell/locales';
import { usePaperlessCategoryStore } from '../../stores/paperless-category.state';

import CategoryDrawer from './category-drawer.vue';

interface CategoryRow {
  id?: string;
  name?: string;
  description?: string;
  parentId?: string;
  documentCount?: number;
  subcategoryCount?: number;
  sortOrder?: number;
  createTime?: string;
}

const categoryStore = usePaperlessCategoryStore();

// Category list
const formOptions: VbenFormProps = {
  collapsed: false,
  showCollapseButton: false,
  submitOnEnter: true,
  schema: [
    {
      component: 'Input',
      fieldName: 'nameFilter',
      label: $t('paperless.page.category.name'),
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
  ],
};

const gridOptions: VxeGridProps<CategoryRow> = {
  height: 'auto',
  stripe: false,
  toolbarConfig: {
    custom: true,
    export: true,
    import: false,
    refresh: true,
    zoom: true,
  },
  exportConfig: {},
  rowConfig: {
    isHover: true,
  },
  treeConfig: {
    rowField: 'id',
    parentField: 'parentId',
    transform: true,
  },

  proxyConfig: {
    ajax: {
      query: async (_, formValues) => {
        const resp = await categoryStore.listCategories(
          { page: 1, pageSize: 1000 },
          {
            nameFilter: formValues?.nameFilter,
          },
        );
        return {
          items: resp.categories ?? resp.items ?? [],
          total: resp.total ?? 0,
        };
      },
    },
  },

  columns: [
    { title: $t('ui.table.seq'), type: 'seq', width: 50 },
    {
      title: $t('paperless.page.category.name'),
      field: 'name',
      minWidth: 200,
      treeNode: true,
    },
    {
      title: $t('paperless.page.category.description'),
      field: 'description',
      minWidth: 200,
    },
    {
      title: $t('paperless.page.category.documentCount'),
      field: 'documentCount',
      width: 120,
    },
    {
      title: $t('paperless.page.category.sortOrder'),
      field: 'sortOrder',
      width: 100,
    },
    {
      title: $t('ui.table.createdAt'),
      field: 'createTime',
      formatter: 'formatDateTime',
      width: 160,
    },
    {
      title: $t('ui.table.action'),
      field: 'action',
      fixed: 'right',
      slots: { default: 'action' },
      width: 150,
    },
  ],
};

const [Grid, gridApi] = useVbenVxeGrid({ gridOptions, formOptions });

// Drawer states
const drawerMode = ref<'create' | 'edit' | 'view'>('view');

const [CategoryDrawerComponent, categoryDrawerApi] = useVbenDrawer({
  connectedComponent: CategoryDrawer,
  onOpenChange(isOpen: boolean) {
    if (!isOpen) {
      gridApi.query();
    }
  },
});

// Category operations
function openCategoryDrawer(
  row: CategoryRow,
  mode: 'create' | 'edit' | 'view',
  parentId?: string,
) {
  drawerMode.value = mode;
  categoryDrawerApi.setData({ category: row, mode, parentId });
  categoryDrawerApi.open();
}

function handleViewCategory(row: CategoryRow) {
  openCategoryDrawer(row, 'view');
}

function handleEditCategory(row: CategoryRow) {
  openCategoryDrawer(row, 'edit');
}

function handleCreateCategory(parentId?: string) {
  openCategoryDrawer({} as CategoryRow, 'create', parentId);
}

function handleCreateSubcategory(row: CategoryRow) {
  openCategoryDrawer({} as CategoryRow, 'create', row.id);
}

async function handleDeleteCategory(row: CategoryRow) {
  if (!row.id) return;

  const hasChildren = (row.documentCount ?? 0) > 0 || (row.subcategoryCount ?? 0) > 0;

  Modal.confirm({
    title: $t('paperless.page.category.delete'),
    content: hasChildren
      ? $t('paperless.page.category.confirmDeleteForce')
      : $t('paperless.page.category.confirmDelete'),
    okText: $t('ui.button.ok'),
    cancelText: $t('ui.button.cancel'),
    onOk: async () => {
      try {
        await categoryStore.deleteCategory(row.id!, hasChildren);
        notification.success({
          message: $t('paperless.page.category.deleteSuccess'),
        });
        await gridApi.query();
      } catch {
        notification.error({ message: $t('ui.notification.delete_failed') });
      }
    },
  });
}

onMounted(() => {
  gridApi.reload();
});
</script>

<template>
  <Page auto-content-height>
    <Grid :table-title="$t('paperless.page.category.title')">
      <template #toolbar-tools>
        <Space>
          <Button type="primary" @click="handleCreateCategory()">
            {{ $t('paperless.page.category.create') }}
          </Button>
        </Space>
      </template>
      <template #action="{ row }">
        <Space>
          <Button
            type="link"
            size="small"
            :icon="h(LucideEye)"
            :title="$t('ui.button.view')"
            @click.stop="handleViewCategory(row)"
          />
          <Button
            type="link"
            size="small"
            :icon="h(LucideFolderPlus)"
            :title="$t('paperless.page.category.create')"
            @click.stop="handleCreateSubcategory(row)"
          />
          <Button
            type="link"
            size="small"
            :icon="h(LucidePencil)"
            :title="$t('ui.button.edit')"
            @click.stop="handleEditCategory(row)"
          />
          <a-popconfirm
            :cancel-text="$t('ui.button.cancel')"
            :ok-text="$t('ui.button.ok')"
            :title="$t('paperless.page.category.confirmDelete')"
            @confirm="handleDeleteCategory(row)"
          >
            <Button
              danger
              type="link"
              size="small"
              :icon="h(LucideTrash)"
              :title="$t('ui.button.delete', { moduleName: '' })"
            />
          </a-popconfirm>
        </Space>
      </template>
    </Grid>

    <CategoryDrawerComponent />
  </Page>
</template>
