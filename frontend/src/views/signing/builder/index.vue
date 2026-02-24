<script lang="ts" setup>
import { ref, onMounted, onBeforeUnmount } from 'vue';
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router';

import { LucideSave, LucideArrowLeft } from 'shell/vben/icons';

import { Button, notification, Spin } from 'ant-design-vue';

import { $t } from 'shell/locales';
import { usePaperlessSigningTemplateStore } from '../../../stores/paperless-signing-template.state';
import type { SigningTemplate } from '../../../stores/paperless-signing-template.state';

import BuilderPdfViewer from './components/BuilderPdfViewer.vue';
import FieldPalette from './components/FieldPalette.vue';
import FieldPropertiesPanel from './components/FieldPropertiesPanel.vue';
import PlacedFieldsList from './components/PlacedFieldsList.vue';
import { useFieldBuilder } from './composables/useFieldBuilder';

const route = useRoute();
const router = useRouter();
const store = usePaperlessSigningTemplateStore();

const templateId = route.params.id as string;
const template = ref<SigningTemplate | null>(null);
const pdfUrl = ref('');
const loading = ref(true);
const saving = ref(false);

const {
  fields,
  selectedFieldId,
  selectedField,
  isDirty,
  loadFields,
  addField,
  updateField,
  removeField,
  selectField,
  toTemplateFields,
  markClean,
} = useFieldBuilder();

onMounted(async () => {
  try {
    const [templateResp, pdfResp] = await Promise.all([
      store.getSigningTemplate(templateId),
      store.getTemplatePdfUrl(templateId),
    ]);
    template.value = templateResp.template ?? null;
    pdfUrl.value = pdfResp.url ?? '';
    if (template.value?.fields) {
      loadFields(template.value.fields);
    }
  } catch (e: any) {
    notification.error({
      message: $t('paperless.page.builder.loadError'),
      description: e?.message,
    });
  } finally {
    loading.value = false;
  }
});

function handleDragStart(type: string, event: DragEvent) {
  event.dataTransfer?.setData('field-type', type);
}

function handleDropField(type: string, pageNumber: number, xPercent: number, yPercent: number) {
  addField(type, pageNumber, xPercent, yPercent);
}

function handleFieldMove(id: string, xPercent: number, yPercent: number) {
  updateField(id, { xPercent, yPercent });
}

function handleFieldResize(id: string, widthPercent: number, heightPercent: number) {
  updateField(id, { widthPercent, heightPercent });
}

function handleFieldSelect(id: string) {
  selectField(id);
}

function handleDeselect() {
  selectField(null);
}

async function handleSave() {
  saving.value = true;
  try {
    await store.updateTemplateFields(templateId, toTemplateFields());
    markClean();
    notification.success({ message: $t('paperless.page.builder.saveSuccess') });
  } catch (e: any) {
    notification.error({
      message: $t('paperless.page.builder.saveError'),
      description: e?.message,
    });
  } finally {
    saving.value = false;
  }
}

function handleBack() {
  router.push('/paperless/signing/templates');
}

// Warn on unsaved changes
onBeforeRouteLeave((_to, _from, next) => {
  if (isDirty.value) {
    const leave = window.confirm($t('paperless.page.builder.unsavedWarning'));
    if (!leave) {
      next(false);
      return;
    }
  }
  next();
});

function handleBeforeUnload(e: BeforeUnloadEvent) {
  if (isDirty.value) {
    e.preventDefault();
  }
}
onMounted(() => window.addEventListener('beforeunload', handleBeforeUnload));
onBeforeUnmount(() => window.removeEventListener('beforeunload', handleBeforeUnload));
</script>

<template>
  <div class="flex h-screen flex-col bg-gray-100 dark:bg-gray-900">
    <!-- Header -->
    <div class="flex items-center justify-between border-b bg-white px-4 py-2.5 dark:bg-gray-800">
      <div class="flex items-center gap-3">
        <Button type="text" size="small" @click="handleBack">
          <template #icon><LucideArrowLeft class="size-4" /></template>
        </Button>
        <h1 class="text-base font-semibold">
          {{ template?.name ?? $t('paperless.page.builder.title') }}
        </h1>
        <span v-if="isDirty" class="rounded bg-orange-100 px-1.5 py-0.5 text-[10px] font-medium text-orange-600">
          {{ $t('paperless.page.builder.unsaved') }}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <Button type="primary" :loading="saving" :disabled="!isDirty" @click="handleSave">
          <template #icon><LucideSave class="size-4" /></template>
          {{ $t('paperless.page.builder.saveFields') }}
        </Button>
      </div>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="flex flex-1 items-center justify-center">
      <Spin size="large" />
    </div>

    <!-- Main content -->
    <div v-else class="flex flex-1 overflow-hidden">
      <!-- Left sidebar: palette + placed fields list -->
      <div class="w-[220px] flex-shrink-0 overflow-y-auto border-r bg-white p-3 dark:bg-gray-800">
        <FieldPalette @drag-start="handleDragStart" />
        <PlacedFieldsList
          :fields="fields"
          :selected-field-id="selectedFieldId"
          @select="handleFieldSelect"
        />
      </div>

      <!-- Center: PDF viewer -->
      <div class="flex-1 overflow-y-auto p-4">
        <div class="mx-auto max-w-3xl">
          <BuilderPdfViewer
            :pdf-url="pdfUrl"
            :fields="fields"
            :selected-field-id="selectedFieldId"
            @field-select="handleFieldSelect"
            @field-move="handleFieldMove"
            @field-resize="handleFieldResize"
            @drop-field="handleDropField"
            @deselect="handleDeselect"
          />
        </div>
      </div>

      <!-- Right sidebar: properties panel -->
      <div
        v-if="selectedField"
        class="w-[260px] flex-shrink-0 overflow-y-auto border-l bg-white p-3 dark:bg-gray-800"
      >
        <FieldPropertiesPanel
          :field="selectedField"
          @update="(updates) => updateField(selectedField!.id, updates)"
          @delete="removeField(selectedField!.id)"
        />
      </div>
    </div>
  </div>
</template>
