<script lang="ts" setup>
import type { VxeGridProps } from 'shell/adapter/vxe-table';

import { h, ref } from 'vue';

import { Page, useVbenDrawer } from 'shell/vben/common-ui';
import {
  LucideEye,
  LucideXCircle,
  LucideDownload,
  LucideFileSignature,
  LucideMail,
  LucideBan,
} from 'shell/vben/icons';

import {
  notification,
  Space,
  Button,
  Tag,
  Modal,
} from 'ant-design-vue';

import { useVbenVxeGrid } from 'shell/adapter/vxe-table';
import { $t } from 'shell/locales';
import { usePaperlessSigningRequestStore } from '../../../stores/paperless-signing-request.state';
import type { SigningRequest, SigningRecipient } from '../../../stores/paperless-signing-request.state';

import RequestDrawer from './request-drawer.vue';

const requestStore = usePaperlessSigningRequestStore();

const formOptions = {
  collapsed: false,
  showCollapseButton: false,
  submitOnEnter: true,
  schema: [
    {
      component: 'Select',
      fieldName: 'status',
      label: $t('paperless.page.signingRequest.status'),
      componentProps: {
        placeholder: $t('ui.placeholder.select'),
        allowClear: true,
        options: [
          { value: 'SIGNING_REQUEST_STATUS_PENDING', label: $t('paperless.page.signingRequest.statusPending') },
          { value: 'SIGNING_REQUEST_STATUS_COMPLETED', label: $t('paperless.page.signingRequest.statusCompleted') },
          { value: 'SIGNING_REQUEST_STATUS_CANCELLED', label: $t('paperless.page.signingRequest.statusCancelled') },
          { value: 'SIGNING_REQUEST_STATUS_EXPIRED', label: $t('paperless.page.signingRequest.statusExpired') },
          { value: 'SIGNING_REQUEST_STATUS_REVOKED', label: $t('paperless.page.signingRequest.statusRevoked') },
        ],
      },
    },
  ],
};

const gridOptions: VxeGridProps<SigningRequest> = {
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
        const resp = await requestStore.listSigningRequests(
          { page: page.currentPage, pageSize: page.pageSize },
          formValues,
        );
        return {
          items: resp.requests ?? [],
          total: resp.total ?? 0,
        };
      },
    },
  },
  columns: [
    { title: $t('ui.table.seq'), type: 'seq', width: 50 },
    {
      title: $t('paperless.page.signingRequest.name'),
      field: 'name',
      minWidth: 200,
      slots: { default: 'name' },
    },
    {
      title: $t('paperless.page.signingRequest.template'),
      field: 'templateName',
      width: 180,
    },
    {
      title: $t('paperless.page.signingRequest.status'),
      field: 'status',
      width: 120,
      slots: { default: 'status' },
    },
    {
      title: $t('paperless.page.signingRequest.signingType'),
      field: 'signingType',
      width: 140,
      slots: { default: 'signingType' },
    },
    {
      title: $t('paperless.page.signingRequest.recipients'),
      field: 'recipients',
      width: 140,
      slots: { default: 'recipients' },
    },
    {
      title: $t('paperless.page.signingRequest.expiresAt'),
      field: 'expiresAt',
      formatter: 'formatDateTime',
      width: 160,
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
      width: 260,
    },
  ],
};

function getStatusColor(status: string | undefined): string {
  switch (status) {
    case 'SIGNING_REQUEST_STATUS_PENDING': return 'processing';
    case 'SIGNING_REQUEST_STATUS_COMPLETED': return 'success';
    case 'SIGNING_REQUEST_STATUS_CANCELLED': return 'default';
    case 'SIGNING_REQUEST_STATUS_EXPIRED': return 'warning';
    case 'SIGNING_REQUEST_STATUS_REVOKED': return 'error';
    case 'SIGNING_REQUEST_STATUS_DRAFT': return 'default';
    default: return 'default';
  }
}

function getStatusLabel(status: string | undefined): string {
  switch (status) {
    case 'SIGNING_REQUEST_STATUS_DRAFT': return $t('paperless.page.signingRequest.statusDraft');
    case 'SIGNING_REQUEST_STATUS_PENDING': return $t('paperless.page.signingRequest.statusPending');
    case 'SIGNING_REQUEST_STATUS_COMPLETED': return $t('paperless.page.signingRequest.statusCompleted');
    case 'SIGNING_REQUEST_STATUS_CANCELLED': return $t('paperless.page.signingRequest.statusCancelled');
    case 'SIGNING_REQUEST_STATUS_EXPIRED': return $t('paperless.page.signingRequest.statusExpired');
    case 'SIGNING_REQUEST_STATUS_REVOKED': return $t('paperless.page.signingRequest.statusRevoked');
    default: return '-';
  }
}

function getRecipientProgress(row: SigningRequest): string {
  const recipients = row.recipients ?? [];
  const completed = recipients.filter(r => r.status === 'SIGNING_RECIPIENT_STATUS_COMPLETED').length;
  return `${completed}/${recipients.length}`;
}

const [Grid, gridApi] = useVbenVxeGrid({ gridOptions, formOptions });

const [RequestDrawerComponent, requestDrawerApi] = useVbenDrawer({
  connectedComponent: RequestDrawer,
  onOpenChange(isOpen: boolean) {
    if (!isOpen) {
      gridApi.query();
    }
  },
});

function handleCreate() {
  requestDrawerApi.setData({ row: {}, mode: 'create' });
  requestDrawerApi.open();
}

function handleView(row: SigningRequest) {
  requestDrawerApi.setData({ row, mode: 'view' });
  requestDrawerApi.open();
}

async function handleCancel(row: SigningRequest) {
  if (!row.id) return;
  Modal.confirm({
    title: $t('paperless.page.signingRequest.cancel'),
    content: $t('paperless.page.signingRequest.confirmCancel'),
    okText: $t('ui.button.ok'),
    cancelText: $t('ui.button.cancel'),
    onOk: async () => {
      try {
        await requestStore.cancelSigningRequest(row.id!);
        notification.success({ message: $t('paperless.page.signingRequest.cancelSuccess') });
        await gridApi.query();
      } catch {
        notification.error({ message: $t('ui.notification.operation_failed') });
      }
    },
  });
}

async function handleRevoke(row: SigningRequest) {
  if (!row.id) return;
  Modal.confirm({
    title: $t('paperless.page.signingRequest.revoke'),
    content: $t('paperless.page.signingRequest.confirmRevoke'),
    okText: $t('ui.button.ok'),
    cancelText: $t('ui.button.cancel'),
    okButtonProps: { danger: true },
    onOk: async () => {
      try {
        await requestStore.revokeSigningRequest(row.id!);
        notification.success({ message: $t('paperless.page.signingRequest.revokeSuccess') });
        await gridApi.query();
      } catch {
        notification.error({ message: $t('ui.notification.operation_failed') });
      }
    },
  });
}

async function handleDownload(row: SigningRequest) {
  if (!row.id) return;
  try {
    const resp = await requestStore.downloadSignedDocument(row.id);
    if (resp.url) {
      const link = document.createElement('a');
      link.href = resp.url;
      link.target = '_blank';
      link.rel = 'noopener';
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      notification.success({ message: $t('paperless.page.signingRequest.downloadStarted') });
    }
  } catch {
    notification.error({ message: $t('ui.notification.operation_failed') });
  }
}

function getResendableRecipients(row: SigningRequest): SigningRecipient[] {
  return (row.recipients ?? []).filter(
    (r) => r.status === 'SIGNING_RECIPIENT_STATUS_PENDING',
  );
}

async function doResend(requestId: string, recipient: SigningRecipient) {
  try {
    await requestStore.resendSigningEmail(requestId, recipient.id!);
    notification.success({
      message: $t('paperless.page.signingRequest.resendSuccess', {
        name: recipient.name || recipient.email,
      }),
    });
  } catch {
    notification.error({ message: $t('ui.notification.operation_failed') });
  }
}

function handleResend(row: SigningRequest) {
  if (!row.id) return;
  const recipients = getResendableRecipients(row);
  if (recipients.length === 0) {
    notification.warning({
      message: $t('paperless.page.signingRequest.noResendableRecipients'),
    });
    return;
  }

  if (recipients.length === 1) {
    doResend(row.id, recipients[0]!);
    return;
  }

  Modal.info({
    title: $t('paperless.page.signingRequest.selectRecipient'),
    content: h(
      'div',
      { style: 'margin-top: 12px' },
      recipients.map((r) =>
        h(
          'div',
          {
            key: r.id,
            style:
              'padding: 8px 12px; margin-bottom: 8px; border: 1px solid #e8e8e8; border-radius: 6px; cursor: pointer; transition: background 0.2s',
            onMouseenter: (e: MouseEvent) =>
              ((e.currentTarget as HTMLElement).style.background = '#f0f5ff'),
            onMouseleave: (e: MouseEvent) =>
              ((e.currentTarget as HTMLElement).style.background = ''),
            onClick: () => {
              Modal.destroyAll();
              doResend(row.id!, r);
            },
          },
          [
            h('div', { style: 'font-weight: 500' }, r.name || r.email || ''),
            h('div', { style: 'font-size: 12px; color: #8c8c8c' }, r.email || ''),
          ],
        ),
      ),
    ),
    okButtonProps: { style: 'display: none' },
  });
}
</script>

<template>
  <Page auto-content-height>
    <Grid :table-title="$t('paperless.page.signingRequest.title')">
      <template #toolbar-tools>
        <Space>
          <Button type="primary" @click="handleCreate">
            {{ $t('paperless.page.signingRequest.create') }}
          </Button>
        </Space>
      </template>
      <template #name="{ row }">
        <div class="flex items-center gap-2">
          <component :is="LucideFileSignature" class="size-4" />
          <span>{{ row.name }}</span>
        </div>
      </template>
      <template #status="{ row }">
        <Tag :color="getStatusColor(row.status)">
          {{ getStatusLabel(row.status) }}
        </Tag>
      </template>
      <template #signingType="{ row }">
        <Tag :color="row.signingType === 'SIGNING_REQUEST_TYPE_INTERNAL' ? 'blue' : 'green'">
          {{ row.signingType === 'SIGNING_REQUEST_TYPE_INTERNAL'
            ? $t('paperless.page.signingRequest.signingTypeInternal')
            : $t('paperless.page.signingRequest.signingTypeExternal') }}
        </Tag>
      </template>
      <template #recipients="{ row }">
        <Tag color="blue">
          {{ getRecipientProgress(row) }} {{ $t('paperless.page.signingRequest.recipients').toLowerCase() }}
        </Tag>
      </template>
      <template #action="{ row }">
        <Space>
          <Button
            type="link"
            size="small"
            :icon="h(LucideEye)"
            :title="$t('paperless.page.signingRequest.view')"
            @click.stop="handleView(row)"
          />
          <Button
            v-if="row.status === 'SIGNING_REQUEST_STATUS_COMPLETED' || row.status === 'SIGNING_REQUEST_STATUS_REVOKED'"
            type="link"
            size="small"
            :icon="h(LucideDownload)"
            :title="$t('paperless.page.signingRequest.download')"
            @click.stop="handleDownload(row)"
          />
          <Button
            v-if="row.status === 'SIGNING_REQUEST_STATUS_COMPLETED'"
            danger
            type="link"
            size="small"
            :icon="h(LucideBan)"
            :title="$t('paperless.page.signingRequest.revoke')"
            @click.stop="handleRevoke(row)"
          />
          <Button
            v-if="row.status === 'SIGNING_REQUEST_STATUS_PENDING'"
            type="link"
            size="small"
            :icon="h(LucideMail)"
            :title="$t('paperless.page.signingRequest.resend')"
            @click.stop="handleResend(row)"
          />
          <Button
            v-if="row.status === 'SIGNING_REQUEST_STATUS_PENDING'"
            danger
            type="link"
            size="small"
            :icon="h(LucideXCircle)"
            :title="$t('paperless.page.signingRequest.cancel')"
            @click.stop="handleCancel(row)"
          />
        </Space>
      </template>
    </Grid>

    <RequestDrawerComponent />
  </Page>
</template>
