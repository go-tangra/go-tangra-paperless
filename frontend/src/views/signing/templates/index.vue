<script lang="ts" setup>
import type { VxeGridProps } from 'shell/adapter/vxe-table';

import { h, ref } from 'vue';
import { useRouter } from 'vue-router';

import { Page, useVbenDrawer } from 'shell/vben/common-ui';
import {
  LucideEye,
  LucideTrash,
  LucidePencil,
  LucideFileSignature,
  LucideLayoutGrid,
} from 'shell/vben/icons';

import {
  notification,
  Space,
  Button,
  Tag,
} from 'ant-design-vue';

import { useVbenVxeGrid } from 'shell/adapter/vxe-table';
import { $t } from 'shell/locales';
import { usePaperlessSigningTemplateStore } from '../../../stores/paperless-signing-template.state';
import type { SigningTemplate } from '../../../stores/paperless-signing-template.state';

import TemplateDrawer from './template-drawer.vue';

const templateStore = usePaperlessSigningTemplateStore();
const router = useRouter();

const formOptions = {
  collapsed: false,
  showCollapseButton: false,
  submitOnEnter: true,
  schema: [
    {
      component: 'Input',
      fieldName: 'nameFilter',
      label: $t('paperless.page.signingTemplate.name'),
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
  ],
};

const gridOptions: VxeGridProps<SigningTemplate> = {
  height: 'auto',
  stripe: false,
  toolbarConfig: {
    custom: true,
    export: true,
    refresh: true,
    zoom: true,
  },
  exportConfig: {},
  rowConfig: { isHover: true },
  pagerConfig: {
    enabled: true,
    pageSize: 20,
    pageSizes: [10, 20, 50, 100],
  },
  proxyConfig: {
    ajax: {
      query: async ({ page }, formValues) => {
        const resp = await templateStore.listSigningTemplates(
          { page: page.currentPage, pageSize: page.pageSize },
          formValues,
        );
        return {
          items: resp.templates ?? [],
          total: resp.total ?? 0,
        };
      },
    },
  },
  columns: [
    { title: $t('ui.table.seq'), type: 'seq', width: 50 },
    {
      title: $t('paperless.page.signingTemplate.name'),
      field: 'name',
      minWidth: 200,
      slots: { default: 'name' },
    },
    {
      title: $t('paperless.page.signingTemplate.fileName'),
      field: 'fileName',
      width: 200,
    },
    {
      title: $t('paperless.page.signingTemplate.fileSize'),
      field: 'fileSize',
      width: 100,
      formatter: ({ cellValue }) => formatFileSize(cellValue),
    },
    {
      title: $t('paperless.page.signingTemplate.fields'),
      field: 'fields',
      width: 120,
      slots: { default: 'fields' },
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
      width: 200,
    },
  ],
};

function formatFileSize(bytes: number | undefined): string {
  if (!bytes) return '-';
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${sizes[i]}`;
}

function getFieldTypeLabel(type: string | undefined): string {
  switch (type) {
    case 'SIGNING_FIELD_TYPE_TEXT': return $t('paperless.page.signingTemplate.typeText');
    case 'SIGNING_FIELD_TYPE_SIGNATURE': return $t('paperless.page.signingTemplate.typeSignature');
    case 'SIGNING_FIELD_TYPE_DATE': return $t('paperless.page.signingTemplate.typeDate');
    case 'SIGNING_FIELD_TYPE_INITIALS': return $t('paperless.page.signingTemplate.typeInitials');
    case 'SIGNING_FIELD_TYPE_CHECKBOX': return $t('paperless.page.signingTemplate.typeCheckbox');
    case 'SIGNING_FIELD_TYPE_EMAIL': return $t('paperless.page.signingTemplate.typeEmail');
    default: return '-';
  }
}

const [Grid, gridApi] = useVbenVxeGrid({ gridOptions, formOptions });

const drawerMode = ref<'create' | 'edit' | 'view'>('view');

const [TemplateDrawerComponent, templateDrawerApi] = useVbenDrawer({
  connectedComponent: TemplateDrawer,
  onOpenChange(isOpen: boolean) {
    if (!isOpen) {
      gridApi.query();
    }
  },
});

function openDrawer(row: SigningTemplate, mode: 'create' | 'edit' | 'view') {
  drawerMode.value = mode;
  templateDrawerApi.setData({ row, mode });
  templateDrawerApi.open();
}

function handleCreate() {
  openDrawer({} as SigningTemplate, 'create');
}

function handleView(row: SigningTemplate) {
  openDrawer(row, 'view');
}

function handleEdit(row: SigningTemplate) {
  openDrawer(row, 'edit');
}

function handleEditFields(row: SigningTemplate) {
  if (!row.id) return;
  router.push(`/paperless/signing/templates/${row.id}/builder`);
}

async function handleDelete(row: SigningTemplate) {
  if (!row.id) return;
  try {
    await templateStore.deleteSigningTemplate(row.id);
    notification.success({ message: $t('paperless.page.signingTemplate.deleteSuccess') });
    await gridApi.query();
  } catch {
    notification.error({ message: $t('ui.notification.delete_failed') });
  }
}
</script>

<template>
  <Page auto-content-height>
    <Grid :table-title="$t('paperless.page.signingTemplate.title')">
      <template #toolbar-tools>
        <Space>
          <Button type="primary" @click="handleCreate">
            {{ $t('paperless.page.signingTemplate.create') }}
          </Button>
        </Space>
      </template>
      <template #name="{ row }">
        <div class="flex items-center gap-2">
          <component :is="LucideFileSignature" class="size-4" />
          <span>{{ row.name }}</span>
        </div>
      </template>
      <template #fields="{ row }">
        <Tag color="blue">
          {{ (row.fields?.length ?? 0) }} {{ $t('paperless.page.signingTemplate.fields').toLowerCase() }}
        </Tag>
      </template>
      <template #action="{ row }">
        <Space>
          <Button
            type="link"
            size="small"
            :icon="h(LucideEye)"
            :title="$t('ui.button.view')"
            @click.stop="handleView(row)"
          />
          <Button
            type="link"
            size="small"
            :icon="h(LucidePencil)"
            :title="$t('ui.button.edit')"
            @click.stop="handleEdit(row)"
          />
          <Button
            type="link"
            size="small"
            :icon="h(LucideLayoutGrid)"
            :title="$t('paperless.page.builder.editFields')"
            @click.stop="handleEditFields(row)"
          />
          <a-popconfirm
            :cancel-text="$t('ui.button.cancel')"
            :ok-text="$t('ui.button.ok')"
            :title="$t('paperless.page.signingTemplate.confirmDelete')"
            @confirm="handleDelete(row)"
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

    <TemplateDrawerComponent />
  </Page>
</template>
