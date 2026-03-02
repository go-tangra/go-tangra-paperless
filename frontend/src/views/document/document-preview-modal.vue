<script lang="ts" setup>
import { ref, watch } from 'vue';

import {
  Modal,
  Tag,
  Button,
  Tabs,
  TabPane,
  Spin,
  Descriptions,
  DescriptionsItem,
  Empty,
  notification,
} from 'ant-design-vue';

import { $t } from 'shell/locales';
import { usePaperlessDocumentStore } from '../../stores/paperless-document.state';

const props = defineProps<{
  open: boolean;
  documentId: string | undefined;
}>();

const emit = defineEmits<{
  (e: 'update:open', value: boolean): void;
}>();

const documentStore = usePaperlessDocumentStore();

const loading = ref(false);
const activeTab = ref('info');
const document = ref<Record<string, any> | null>(null);

watch(
  () => props.open,
  async (isOpen) => {
    if (isOpen && props.documentId) {
      activeTab.value = 'info';
      loading.value = true;
      try {
        const resp = await documentStore.getDocument(props.documentId);
        document.value = resp.document ?? resp ?? null;
      } catch (e) {
        console.error('Failed to load document:', e);
        document.value = null;
      } finally {
        loading.value = false;
      }
    } else if (!isOpen) {
      document.value = null;
    }
  },
);

function handleClose() {
  emit('update:open', false);
}

function getProcessingStatusColor(status: string | undefined): string {
  switch (status) {
    case 'completed': return 'green';
    case 'processing': return 'blue';
    case 'pending': return 'orange';
    case 'failed': return 'red';
    case 'skipped': return 'default';
    default: return 'default';
  }
}

function getProcessingStatusLabel(status: string | undefined): string {
  switch (status) {
    case 'completed': return $t('paperless.page.document.processingCompleted');
    case 'processing': return $t('paperless.page.document.processingInProgress');
    case 'pending': return $t('paperless.page.document.processingPending');
    case 'failed': return $t('paperless.page.document.processingFailed');
    case 'skipped': return $t('paperless.page.document.processingSkipped');
    default: return status || '-';
  }
}

function getStatusColor(status: string | undefined): string {
  switch (status) {
    case 'DOCUMENT_STATUS_ACTIVE': return 'green';
    case 'DOCUMENT_STATUS_ARCHIVED': return 'orange';
    case 'DOCUMENT_STATUS_DELETED': return 'red';
    default: return 'default';
  }
}

function getStatusLabel(status: string | undefined): string {
  switch (status) {
    case 'DOCUMENT_STATUS_ACTIVE': return $t('paperless.page.document.statusActive');
    case 'DOCUMENT_STATUS_ARCHIVED': return $t('paperless.page.document.statusArchived');
    case 'DOCUMENT_STATUS_DELETED': return $t('paperless.page.document.statusDeleted');
    default: return '-';
  }
}

function getSourceLabel(source: string | undefined): string {
  switch (source) {
    case 'DOCUMENT_SOURCE_UPLOAD': return $t('paperless.page.document.sourceUpload');
    case 'DOCUMENT_SOURCE_EMAIL': return $t('paperless.page.document.sourceEmail');
    default: return '-';
  }
}

function formatFileSize(bytes: number | undefined): string {
  if (!bytes) return '-';
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${sizes[i]}`;
}

function formatDateTime(value: string | undefined): string {
  if (!value) return '-';
  try { return new Date(value).toLocaleString(); }
  catch { return value; }
}

async function copyContent() {
  const text = document.value?.contentText;
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
    notification.success({ message: $t('paperless.page.document.copiedToClipboard') });
  } catch {
    notification.error({ message: 'Failed to copy' });
  }
}

const extractedMetadataEntries = ref<Array<{ key: string; value: string }>>([]);

watch(
  () => document.value?.extractedMetadata,
  (metadata) => {
    if (metadata && typeof metadata === 'object') {
      extractedMetadataEntries.value = Object.entries(metadata).map(
        ([key, value]) => ({ key, value: String(value) }),
      );
    } else {
      extractedMetadataEntries.value = [];
    }
  },
);
</script>

<template>
  <Modal :open="props.open" :title="undefined" :footer="null" :width="800" @cancel="handleClose">
    <Spin :spinning="loading">
      <template v-if="document">
        <div class="mb-4 flex items-center gap-3">
          <h3 class="m-0 text-lg font-semibold">{{ document.name || '-' }}</h3>
          <Tag :color="getProcessingStatusColor(document.processingStatus)">
            {{ getProcessingStatusLabel(document.processingStatus) }}
          </Tag>
        </div>
        <Tabs v-model:activeKey="activeTab">
          <TabPane key="info" :tab="$t('paperless.page.document.info')">
            <Descriptions bordered :column="1" size="small">
              <DescriptionsItem :label="$t('paperless.page.document.name')">{{ document.name || '-' }}</DescriptionsItem>
              <DescriptionsItem :label="$t('paperless.page.document.fileName')">{{ document.fileName || '-' }}</DescriptionsItem>
              <DescriptionsItem :label="$t('paperless.page.document.fileSize')">{{ formatFileSize(document.fileSize) }}</DescriptionsItem>
              <DescriptionsItem :label="$t('paperless.page.document.mimeType')">{{ document.mimeType || '-' }}</DescriptionsItem>
              <DescriptionsItem :label="$t('paperless.page.document.checksum')">
                <code v-if="document.checksum" class="text-xs">{{ document.checksum }}</code>
                <span v-else>-</span>
              </DescriptionsItem>
              <DescriptionsItem :label="$t('paperless.page.document.source')">{{ getSourceLabel(document.source) }}</DescriptionsItem>
              <DescriptionsItem :label="$t('paperless.page.document.status')">
                <Tag :color="getStatusColor(document.status)">{{ getStatusLabel(document.status) }}</Tag>
              </DescriptionsItem>
              <DescriptionsItem :label="$t('paperless.page.document.processingStatus')">
                <Tag :color="getProcessingStatusColor(document.processingStatus)">{{ getProcessingStatusLabel(document.processingStatus) }}</Tag>
              </DescriptionsItem>
              <DescriptionsItem :label="$t('ui.table.createdAt')">{{ formatDateTime(document.createdAt ?? document.createTime) }}</DescriptionsItem>
              <DescriptionsItem :label="$t('ui.table.updatedAt')">{{ formatDateTime(document.updatedAt ?? document.updateTime) }}</DescriptionsItem>
            </Descriptions>
          </TabPane>
          <TabPane key="content" :tab="$t('paperless.page.document.contentText')">
            <template v-if="document.contentText">
              <div class="mb-2 flex justify-end">
                <Button size="small" @click="copyContent">{{ $t('paperless.page.document.copyContent') }}</Button>
              </div>
              <pre class="max-h-[500px] overflow-auto rounded bg-accent p-4 text-sm whitespace-pre-wrap">{{ document.contentText }}</pre>
            </template>
            <Empty v-else :description="$t('paperless.page.document.noContentExtracted')" />
          </TabPane>
          <TabPane key="metadata" :tab="$t('paperless.page.document.extractedMetadata')">
            <template v-if="extractedMetadataEntries.length > 0">
              <Descriptions bordered :column="1" size="small">
                <DescriptionsItem v-for="entry in extractedMetadataEntries" :key="entry.key" :label="entry.key">{{ entry.value }}</DescriptionsItem>
              </Descriptions>
            </template>
            <Empty v-else :description="$t('paperless.page.document.noMetadata')" />
          </TabPane>
        </Tabs>
      </template>
      <Empty v-else-if="!loading" />
    </Spin>
  </Modal>
</template>
