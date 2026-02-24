<script lang="ts" setup>
import type { VxeGridProps } from 'shell/adapter/vxe-table';

import { h, ref, computed, onMounted } from 'vue';

import { Page, useVbenDrawer, type VbenFormProps } from 'shell/vben/common-ui';
import {
  LucideEye,
  LucideTrash,
  LucidePencil,
  LucideDownload,
  LucideFile,
  LucideFolder,
  LucideFolderOpen,
  LucideKey,
  LucideUpload,
  LucideShare2,
  LucidePlus,
  LucideX,
} from 'shell/vben/icons';

import {
  notification,
  Tree,
  Dropdown,
  Menu,
  MenuItem,
  Space,
  Button,
  Modal,
  Tag,
  Select,
  SelectOption,
  Divider,
} from 'ant-design-vue';
import type { Key } from 'ant-design-vue/es/_util/type';

import { useVbenVxeGrid } from 'shell/adapter/vxe-table';
import { $t } from 'shell/locales';
import { useAccessStore } from 'shell/vben/stores';
import type { CreateSharePolicyInput, SharePolicyType, SharePolicyMethod } from '../../types';
import { usePaperlessCategoryStore } from '../../stores/paperless-category.state';
import { usePaperlessDocumentStore } from '../../stores/paperless-document.state';

import DocumentDrawer from './document-drawer.vue';
import DocumentPreviewModal from './document-preview-modal.vue';
import PermissionDrawer from '../permission/permission-drawer.vue';
import CategoryDrawer from '../category/category-drawer.vue';

interface DocumentRow {
  id?: string;
  name?: string;
  description?: string;
  categoryId?: string;
  fileName?: string;
  mimeType?: string;
  fileSize?: number;
  status?: string;
  source?: string;
  createTime?: string;
}

const categoryStore = usePaperlessCategoryStore();
const documentStore = usePaperlessDocumentStore();

async function createShare(data: Record<string, unknown>) {
  const token = (useAccessStore() as any).accessToken;
  const res = await fetch('/admin/v1/modules/sharing/v1/shares', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(data),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.message || `HTTP ${res.status}`);
  }
  return res.json();
}

// Category tree node interface (matches store return type)
interface CategoryTreeNode {
  category?: {
    id?: string;
    name?: string;
    documentCount?: number;
    subcategoryCount?: number;
  };
  children?: CategoryTreeNode[];
}

// Category tree state
const categoryTree = ref<CategoryTreeNode[]>([]);
const selectedCategoryId = ref<string | undefined>(undefined);
const expandedKeys = ref<string[]>([]);
const loadingTree = ref(false);

// Convert category tree to ant-design tree data format
interface TreeNode {
  key: string;
  title: string;
  children?: TreeNode[];
  isLeaf?: boolean;
  raw: CategoryTreeNode;
}

function convertToTreeData(nodes: CategoryTreeNode[]): TreeNode[] {
  return nodes.map((node) => ({
    key: node.category?.id ?? '',
    title: node.category?.name ?? '',
    children: node.children ? convertToTreeData(node.children) : undefined,
    isLeaf: !node.children || node.children.length === 0,
    raw: node,
  }));
}

const treeData = computed(() => convertToTreeData(categoryTree.value));

// Load category tree
async function loadCategoryTree() {
  loadingTree.value = true;
  try {
    const resp = await categoryStore.getCategoryTree(undefined, undefined, true);
    categoryTree.value = resp.roots ?? resp.nodes ?? [];
  } catch (e) {
    console.error('Failed to load category tree:', e);
    notification.error({ message: $t('ui.notification.load_failed') });
  } finally {
    loadingTree.value = false;
  }
}

onMounted(() => {
  loadCategoryTree();
});

// Handle category selection
function handleCategorySelect(keys: Key[]) {
  if (keys.length > 0) {
    selectedCategoryId.value = String(keys[0]);
    gridApi.reload();
  } else {
    selectedCategoryId.value = undefined;
    gridApi.reload();
  }
}

// Document list
const formOptions: VbenFormProps = {
  collapsed: false,
  showCollapseButton: false,
  submitOnEnter: true,
  schema: [
    {
      component: 'Input',
      fieldName: 'query',
      label: $t('paperless.page.search.title'),
      componentProps: {
        placeholder: $t('paperless.page.search.placeholder'),
        allowClear: true,
      },
    },
  ],
};

const gridOptions: VxeGridProps<DocumentRow> = {
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
  pagerConfig: {
    enabled: true,
    pageSize: 20,
    pageSizes: [10, 20, 50, 100],
  },

  proxyConfig: {
    ajax: {
      query: async ({ page }, formValues) => {
        const paging = { page: page.currentPage, pageSize: page.pageSize };
        const searchQuery = formValues?.query?.trim();

        if (searchQuery) {
          const resp = await documentStore.searchDocuments(
            searchQuery,
            paging,
            {
              categoryId: selectedCategoryId.value,
              includeSubcategories: true,
            },
          );
          return {
            items: resp.documents ?? resp.items ?? [],
            total: resp.total ?? 0,
          };
        }

        const resp = await documentStore.listDocuments(paging, {
          categoryId: selectedCategoryId.value,
        });
        return {
          items: resp.documents ?? resp.items ?? [],
          total: resp.total ?? 0,
        };
      },
    },
  },

  columns: [
    { title: $t('ui.table.seq'), type: 'seq', width: 50 },
    {
      title: $t('paperless.page.document.name'),
      field: 'name',
      minWidth: 200,
      slots: { default: 'name' },
    },
    { title: $t('paperless.page.document.fileName'), field: 'fileName', width: 150 },
    {
      title: $t('paperless.page.document.fileSize'),
      field: 'fileSize',
      width: 100,
      formatter: ({ cellValue }) => formatFileSize(cellValue),
    },
    {
      title: $t('paperless.page.document.source'),
      field: 'source',
      width: 100,
      slots: { default: 'source' },
    },
    {
      title: $t('paperless.page.document.status'),
      field: 'status',
      width: 100,
      slots: { default: 'status' },
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
      width: 220,
    },
  ],
};

function formatFileSize(bytes: number | undefined): string {
  if (!bytes) return '-';
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${sizes[i]}`;
}

const [Grid, gridApi] = useVbenVxeGrid({ gridOptions, formOptions });

// Preview modal state
const previewVisible = ref(false);
const previewDocumentId = ref<string>();

// Drawer states
const drawerMode = ref<'create' | 'edit' | 'view'>('view');
const categoryDrawerMode = ref<'create' | 'edit'>('create');

const [DocumentDrawerComponent, documentDrawerApi] = useVbenDrawer({
  connectedComponent: DocumentDrawer,
  onOpenChange(isOpen: boolean) {
    if (!isOpen) {
      gridApi.query();
    }
  },
});

const [CategoryDrawerComponent, categoryDrawerApi] = useVbenDrawer({
  connectedComponent: CategoryDrawer,
  onOpenChange(isOpen: boolean) {
    if (!isOpen) {
      loadCategoryTree();
    }
  },
});

const [PermissionDrawerComponent, permissionDrawerApi] = useVbenDrawer({
  connectedComponent: PermissionDrawer,
});

// Document operations
function openDocumentDrawer(
  row: DocumentRow,
  mode: 'create' | 'edit' | 'view',
) {
  drawerMode.value = mode;
  documentDrawerApi.setData({ row, mode, categoryId: selectedCategoryId.value });
  documentDrawerApi.open();
}

function handleViewDocument(row: DocumentRow) {
  previewDocumentId.value = row.id;
  previewVisible.value = true;
}

function handleEditDocument(row: DocumentRow) {
  openDocumentDrawer(row, 'edit');
}

function handleCreateDocument() {
  openDocumentDrawer({} as DocumentRow, 'create');
}

async function handleDeleteDocument(row: DocumentRow) {
  if (!row.id) return;
  try {
    // If document is already soft-deleted, permanently delete it
    const permanent = row.status === 'DOCUMENT_STATUS_DELETED';
    await documentStore.deleteDocument(row.id, permanent);
    notification.success({ message: $t('paperless.page.document.deleteSuccess') });
    await gridApi.query();
  } catch {
    notification.error({ message: $t('ui.notification.delete_failed') });
  }
}

async function handleDownloadDocument(row: DocumentRow) {
  if (!row.id) return;
  try {
    const resp = await documentStore.downloadDocument(row.id);
    if (resp.content) {
      const byteCharacters = atob(resp.content);
      const byteNumbers = new Uint8Array(byteCharacters.length);
      for (let i = 0; i < byteCharacters.length; i++) {
        byteNumbers[i] = byteCharacters.charCodeAt(i);
      }
      const blob = new Blob([byteNumbers], {
        type: resp.mimeType || 'application/octet-stream',
      });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = resp.fileName || row.fileName || 'download';
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
      notification.success({ message: $t('paperless.page.document.downloadStarted') });
    }
  } catch {
    notification.error({ message: $t('ui.notification.operation_failed') });
  }
}

function handleViewPermissions(row: DocumentRow) {
  permissionDrawerApi.setData({
    resourceType: 'RESOURCE_TYPE_DOCUMENT',
    resourceId: row.id,
    resourceName: row.name,
  });
  permissionDrawerApi.open();
}

// Category operations
function handleCreateCategory(parentId?: string) {
  categoryDrawerMode.value = 'create';
  categoryDrawerApi.setData({ mode: 'create', parentId });
  categoryDrawerApi.open();
}

function handleEditCategory(node: TreeNode) {
  categoryDrawerMode.value = 'edit';
  categoryDrawerApi.setData({ mode: 'edit', category: node.raw.category });
  categoryDrawerApi.open();
}

async function handleDeleteCategory(node: TreeNode) {
  const category = node.raw.category;
  if (!category?.id) return;

  const hasChildren =
    (category.documentCount ?? 0) > 0 || (category.subcategoryCount ?? 0) > 0;

  Modal.confirm({
    title: $t('paperless.page.category.delete'),
    content: hasChildren
      ? $t('paperless.page.category.confirmDeleteForce')
      : $t('paperless.page.category.confirmDelete'),
    okText: $t('ui.button.ok'),
    cancelText: $t('ui.button.cancel'),
    onOk: async () => {
      try {
        await categoryStore.deleteCategory(category.id!, hasChildren);
        notification.success({
          message: $t('paperless.page.category.deleteSuccess'),
        });
        if (selectedCategoryId.value === category.id) {
          selectedCategoryId.value = undefined;
        }
        await loadCategoryTree();
        await gridApi.query();
      } catch {
        notification.error({ message: $t('ui.notification.delete_failed') });
      }
    },
  });
}

function handleViewCategoryPermissions(node: TreeNode) {
  permissionDrawerApi.setData({
    resourceType: 'RESOURCE_TYPE_CATEGORY',
    resourceId: node.raw.category?.id,
    resourceName: node.raw.category?.name,
  });
  permissionDrawerApi.open();
}

// Get selected category name
const selectedCategoryName = computed(() => {
  if (!selectedCategoryId.value) return $t('paperless.page.category.rootCategory');
  const findCategory = (
    nodes: CategoryTreeNode[],
  ): string | undefined => {
    for (const node of nodes) {
      if (node.category?.id === selectedCategoryId.value) {
        return node.category?.name;
      }
      if (node.children) {
        const found = findCategory(node.children);
        if (found) return found;
      }
    }
    return undefined;
  };
  return findCategory(categoryTree.value) ?? $t('paperless.page.category.rootCategory');
});

// Source display
function getSourceColor(source: string | undefined): string {
  switch (source) {
    case 'DOCUMENT_SOURCE_UPLOAD':
      return 'blue';
    case 'DOCUMENT_SOURCE_EMAIL':
      return 'green';
    default:
      return 'default';
  }
}

function getSourceLabel(source: string | undefined): string {
  switch (source) {
    case 'DOCUMENT_SOURCE_UPLOAD':
      return $t('paperless.page.document.sourceUpload');
    case 'DOCUMENT_SOURCE_EMAIL':
      return $t('paperless.page.document.sourceEmail');
    default:
      return '-';
  }
}

// Status display
function getStatusColor(status: string | undefined): string {
  switch (status) {
    case 'DOCUMENT_STATUS_ACTIVE':
      return 'green';
    case 'DOCUMENT_STATUS_ARCHIVED':
      return 'orange';
    case 'DOCUMENT_STATUS_DELETED':
      return 'red';
    default:
      return 'default';
  }
}

function getStatusLabel(status: string | undefined): string {
  switch (status) {
    case 'DOCUMENT_STATUS_ACTIVE':
      return $t('paperless.page.document.statusActive');
    case 'DOCUMENT_STATUS_ARCHIVED':
      return $t('paperless.page.document.statusArchived');
    case 'DOCUMENT_STATUS_DELETED':
      return $t('paperless.page.document.statusDeleted');
    default:
      return '-';
  }
}

// Share document
const shareModalVisible = ref(false);
const shareDocumentRow = ref<DocumentRow | null>(null);
const shareEmail = ref('');
const shareMessage = ref('');
const shareLoading = ref(false);
const sharePolicies = ref<CreateSharePolicyInput[]>([]);
const showPolicyForm = ref(false);
const policyType = ref<SharePolicyType>('SHARE_POLICY_TYPE_WHITELIST');
const policyMethod = ref<SharePolicyMethod>('SHARE_POLICY_METHOD_IP');
const policyValue = ref('');
const policyReason = ref('');

const policyMethodOptions: { value: SharePolicyMethod; label: string; placeholder: string }[] = [
  { value: 'SHARE_POLICY_METHOD_IP', label: 'IP Address', placeholder: 'e.g. 192.168.1.1' },
  { value: 'SHARE_POLICY_METHOD_NETWORK', label: 'Network (CIDR)', placeholder: 'e.g. 10.1.111.0/24' },
  { value: 'SHARE_POLICY_METHOD_MAC', label: 'MAC Address', placeholder: 'e.g. AA:BB:CC:DD:EE:FF' },
  { value: 'SHARE_POLICY_METHOD_REGION', label: 'Region', placeholder: 'e.g. US, DE, BG' },
  { value: 'SHARE_POLICY_METHOD_TIME', label: 'Time Window', placeholder: 'e.g. 09:00-17:00' },
  { value: 'SHARE_POLICY_METHOD_DEVICE', label: 'Device', placeholder: 'e.g. mobile, desktop' },
];

const currentPlaceholder = computed(() => {
  return policyMethodOptions.find(o => o.value === policyMethod.value)?.placeholder || '';
});

function handleShareDocument(row: DocumentRow) {
  shareDocumentRow.value = row;
  shareEmail.value = '';
  shareMessage.value = '';
  sharePolicies.value = [];
  showPolicyForm.value = false;
  shareModalVisible.value = true;
}

function handleAddPolicy() {
  if (!policyValue.value) return;
  sharePolicies.value.push({
    type: policyType.value,
    method: policyMethod.value,
    value: policyValue.value,
    reason: policyReason.value || undefined,
  });
  policyValue.value = '';
  policyReason.value = '';
  showPolicyForm.value = false;
}

function handleRemovePolicy(index: number) {
  sharePolicies.value.splice(index, 1);
}

function getPolicyTypeLabel(type: SharePolicyType) {
  return type === 'SHARE_POLICY_TYPE_WHITELIST' ? 'Allow' : 'Deny';
}

function getPolicyMethodLabel(method: SharePolicyMethod) {
  return policyMethodOptions.find(o => o.value === method)?.label || method;
}

async function handleShareSubmit() {
  if (!shareDocumentRow.value?.id || !shareEmail.value) return;
  shareLoading.value = true;
  try {
    await createShare({
      resourceType: 'RESOURCE_TYPE_DOCUMENT',
      resourceId: shareDocumentRow.value.id,
      recipientEmail: shareEmail.value,
      message: shareMessage.value || undefined,
      policies: sharePolicies.value.length > 0 ? sharePolicies.value : undefined,
    });
    notification.success({ message: 'Share link created and email sent' });
    shareModalVisible.value = false;
  } catch {
    notification.error({ message: 'Failed to create share' });
  } finally {
    shareLoading.value = false;
  }
}
</script>

<template>
  <Page auto-content-height>
    <div class="flex h-full gap-4">
      <!-- Category Tree Panel -->
      <div class="w-64 shrink-0 overflow-auto rounded border bg-card p-4">
        <div class="mb-4 flex items-center justify-between">
          <span class="font-semibold">{{ $t('paperless.page.category.title') }}</span>
          <Button
            type="primary"
            size="small"
            @click="handleCreateCategory(selectedCategoryId)"
          >
            {{ $t('ui.button.create', { moduleName: '' }) }}
          </Button>
        </div>

        <!-- Root category item -->
        <div
          class="mb-2 flex cursor-pointer items-center gap-2 rounded p-2 hover:bg-accent"
          :class="{ 'bg-accent': !selectedCategoryId }"
          @click="
            selectedCategoryId = undefined;
            gridApi.reload();
          "
        >
          <component :is="LucideFolderOpen" class="size-4" />
          <span>{{ $t('paperless.page.category.rootCategory') }}</span>
        </div>

        <Tree
          v-if="treeData.length > 0"
          v-model:expanded-keys="expandedKeys"
          :tree-data="treeData"
          :selectable="true"
          :selected-keys="selectedCategoryId ? [selectedCategoryId] : []"
          block-node
          @select="handleCategorySelect"
        >
          <template #title="{ title, key, raw }">
            <Dropdown :trigger="['contextmenu']">
              <div class="flex items-center gap-2">
                <component
                  :is="expandedKeys.includes(key) ? LucideFolderOpen : LucideFolder"
                  class="size-4"
                />
                <span>{{ title }}</span>
                <span v-if="raw?.category?.documentCount" class="text-muted-foreground text-xs">
                  ({{ raw.category.documentCount }})
                </span>
              </div>
              <template #overlay>
                <Menu>
                  <MenuItem key="create" @click="handleCreateCategory(key)">
                    {{ $t('paperless.page.category.create') }}
                  </MenuItem>
                  <MenuItem key="edit" @click="handleEditCategory({ key, title, raw })">
                    {{ $t('paperless.page.category.edit') }}
                  </MenuItem>
                  <MenuItem
                    key="permissions"
                    @click="handleViewCategoryPermissions({ key, title, raw })"
                  >
                    {{ $t('paperless.page.permission.title') }}
                  </MenuItem>
                  <MenuItem
                    key="delete"
                    danger
                    @click="handleDeleteCategory({ key, title, raw })"
                  >
                    {{ $t('paperless.page.category.delete') }}
                  </MenuItem>
                </Menu>
              </template>
            </Dropdown>
          </template>
        </Tree>

        <div v-else-if="!loadingTree" class="text-muted-foreground text-center text-sm">
          {{ $t('ui.text.no_data') }}
        </div>
      </div>

      <!-- Document List Panel -->
      <div class="flex-1 overflow-hidden">
        <Grid :table-title="`${$t('paperless.page.document.title')} - ${selectedCategoryName}`">
          <template #toolbar-tools>
            <Space>
              <Button type="primary" @click="handleCreateDocument">
                {{ $t('paperless.page.document.upload') }}
              </Button>
            </Space>
          </template>
          <template #name="{ row }">
            <div class="flex items-center gap-2">
              <component :is="LucideFile" class="size-4" />
              <span>{{ row.name }}</span>
            </div>
          </template>
          <template #source="{ row }">
            <Tag :color="getSourceColor(row.source)">
              {{ getSourceLabel(row.source) }}
            </Tag>
          </template>
          <template #status="{ row }">
            <Tag :color="getStatusColor(row.status)">
              {{ getStatusLabel(row.status) }}
            </Tag>
          </template>
          <template #action="{ row }">
            <Space>
              <Button
                type="link"
                size="small"
                :icon="h(LucideEye)"
                :title="$t('ui.button.view')"
                @click.stop="handleViewDocument(row)"
              />
              <Button
                type="link"
                size="small"
                :icon="h(LucideDownload)"
                :title="$t('paperless.page.document.download')"
                @click.stop="handleDownloadDocument(row)"
              />
              <Button
                type="link"
                size="small"
                :icon="h(LucidePencil)"
                :title="$t('ui.button.edit')"
                @click.stop="handleEditDocument(row)"
              />
              <Button
                type="link"
                size="small"
                :icon="h(LucideShare2)"
                title="Share"
                @click.stop="handleShareDocument(row)"
              />
              <Button
                type="link"
                size="small"
                :icon="h(LucideKey)"
                :title="$t('paperless.page.permission.title')"
                @click.stop="handleViewPermissions(row)"
              />
              <a-popconfirm
                :cancel-text="$t('ui.button.cancel')"
                :ok-text="$t('ui.button.ok')"
                :title="$t('paperless.page.document.confirmDelete')"
                @confirm="handleDeleteDocument(row)"
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
      </div>
    </div>

    <DocumentDrawerComponent />
    <CategoryDrawerComponent />
    <PermissionDrawerComponent />
    <DocumentPreviewModal
      v-model:open="previewVisible"
      :document-id="previewDocumentId"
    />

    <!-- Share Document Modal -->
    <Modal
      v-model:open="shareModalVisible"
      title="Share Document"
      :confirm-loading="shareLoading"
      :width="560"
      @ok="handleShareSubmit"
    >
      <div v-if="shareDocumentRow" style="margin-bottom: 16px">
        Sharing: <strong>{{ shareDocumentRow.name }}</strong>
      </div>
      <div style="margin-bottom: 12px">
        <label style="display: block; margin-bottom: 4px; font-weight: 500">Recipient Email *</label>
        <a-input
          v-model:value="shareEmail"
          placeholder="Enter recipient email"
          type="email"
        />
      </div>
      <div style="margin-bottom: 12px">
        <label style="display: block; margin-bottom: 4px; font-weight: 500">Message (optional)</label>
        <a-textarea
          v-model:value="shareMessage"
          :rows="2"
          placeholder="Optional message to include in the email"
        />
      </div>

      <Divider style="margin: 12px 0 8px">Access Restrictions</Divider>

      <!-- Existing policies list -->
      <div v-if="sharePolicies.length > 0" style="margin-bottom: 8px">
        <div
          v-for="(policy, idx) in sharePolicies"
          :key="idx"
          style="display: flex; align-items: center; gap: 8px; padding: 6px 8px; background: #f5f5f5; border-radius: 4px; margin-bottom: 4px; font-size: 13px"
        >
          <a-tag :color="policy.type === 'SHARE_POLICY_TYPE_WHITELIST' ? 'green' : 'red'" style="margin: 0">
            {{ getPolicyTypeLabel(policy.type) }}
          </a-tag>
          <span style="font-weight: 500">{{ getPolicyMethodLabel(policy.method) }}</span>
          <span style="flex: 1; color: #666">{{ policy.value }}</span>
          <span v-if="policy.reason" style="color: #999; font-size: 12px">({{ policy.reason }})</span>
          <Button type="text" size="small" danger :icon="h(LucideX)" @click="handleRemovePolicy(idx)" />
        </div>
      </div>

      <!-- Add policy form -->
      <div v-if="showPolicyForm" style="border: 1px solid #d9d9d9; border-radius: 6px; padding: 12px; margin-bottom: 8px">
        <div style="display: flex; gap: 8px; margin-bottom: 8px">
          <Select v-model:value="policyType" style="width: 130px" size="small">
            <SelectOption value="SHARE_POLICY_TYPE_WHITELIST">Allow</SelectOption>
            <SelectOption value="SHARE_POLICY_TYPE_BLACKLIST">Deny</SelectOption>
          </Select>
          <Select v-model:value="policyMethod" style="width: 160px" size="small">
            <SelectOption
              v-for="opt in policyMethodOptions"
              :key="opt.value"
              :value="opt.value"
            >{{ opt.label }}</SelectOption>
          </Select>
        </div>
        <div style="display: flex; gap: 8px; margin-bottom: 8px">
          <a-input
            v-model:value="policyValue"
            size="small"
            :placeholder="currentPlaceholder"
            style="flex: 1"
            @press-enter="handleAddPolicy"
          />
          <a-input
            v-model:value="policyReason"
            size="small"
            placeholder="Reason (optional)"
            style="flex: 1"
          />
        </div>
        <div style="display: flex; gap: 8px; justify-content: flex-end">
          <Button size="small" @click="showPolicyForm = false">Cancel</Button>
          <Button size="small" type="primary" :disabled="!policyValue" @click="handleAddPolicy">Add</Button>
        </div>
      </div>

      <Button
        v-if="!showPolicyForm"
        type="dashed"
        size="small"
        block
        :icon="h(LucidePlus)"
        @click="showPolicyForm = true"
      >
        Add Restriction
      </Button>
    </Modal>
  </Page>
</template>
