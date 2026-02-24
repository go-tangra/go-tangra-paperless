import { ref, computed } from 'vue';

import type { SigningTemplateField, SigningFieldType } from '../../../../stores/paperless-signing-template.state';

export interface BuilderField extends SigningTemplateField {
  id: string;
}

const FIELD_TYPE_DEFAULTS: Record<string, { widthPercent: number; heightPercent: number }> = {
  text: { widthPercent: 20, heightPercent: 3 },
  signature: { widthPercent: 20, heightPercent: 6 },
  date: { widthPercent: 15, heightPercent: 3 },
  initials: { widthPercent: 10, heightPercent: 5 },
  checkbox: { widthPercent: 3, heightPercent: 3 },
  email: { widthPercent: 20, heightPercent: 3 },
};

function fieldTypeToShort(type?: SigningFieldType): string {
  switch (type) {
    case 'SIGNING_FIELD_TYPE_TEXT': return 'text';
    case 'SIGNING_FIELD_TYPE_SIGNATURE': return 'signature';
    case 'SIGNING_FIELD_TYPE_DATE': return 'date';
    case 'SIGNING_FIELD_TYPE_INITIALS': return 'initials';
    case 'SIGNING_FIELD_TYPE_CHECKBOX': return 'checkbox';
    case 'SIGNING_FIELD_TYPE_EMAIL': return 'email';
    default: return 'text';
  }
}

export function useFieldBuilder() {
  const fields = ref<BuilderField[]>([]);
  const selectedFieldId = ref<string | null>(null);
  const isDirty = ref(false);
  let nextId = 1;

  const selectedField = computed(() =>
    fields.value.find((f) => f.id === selectedFieldId.value) ?? null,
  );

  function loadFields(templateFields: SigningTemplateField[]) {
    fields.value = templateFields.map((f) => ({
      ...f,
      id: f.id ?? `field-${nextId++}`,
    })) as BuilderField[];
    // Set nextId past existing field IDs
    for (const f of fields.value) {
      const match = f.id.match(/^field-(\d+)$/);
      if (match) {
        const n = parseInt(match[1]!, 10);
        if (n >= nextId) nextId = n + 1;
      }
    }
    isDirty.value = false;
    selectedFieldId.value = null;
  }

  function addField(type: string, pageNumber: number, xPercent: number, yPercent: number): BuilderField {
    const defaults = FIELD_TYPE_DEFAULTS[type] ?? FIELD_TYPE_DEFAULTS.text!;
    const protoType = (`SIGNING_FIELD_TYPE_${type.toUpperCase()}`) as SigningFieldType;
    const id = `field-${nextId++}`;
    const field: BuilderField = {
      id,
      name: `${type.charAt(0).toUpperCase()}${type.slice(1)} ${fields.value.filter((f) => fieldTypeToShort(f.type) === type).length + 1}`,
      type: protoType,
      required: false,
      pageNumber,
      xPercent,
      yPercent,
      widthPercent: defaults.widthPercent,
      heightPercent: defaults.heightPercent,
      prefillStage: 0,
      recipientIndex: 0,
    };
    fields.value.push(field);
    selectedFieldId.value = id;
    isDirty.value = true;
    return field;
  }

  function updateField(id: string, updates: Partial<BuilderField>) {
    const idx = fields.value.findIndex((f) => f.id === id);
    if (idx >= 0) {
      fields.value[idx] = { ...fields.value[idx]!, ...updates };
      isDirty.value = true;
    }
  }

  function removeField(id: string) {
    fields.value = fields.value.filter((f) => f.id !== id);
    if (selectedFieldId.value === id) {
      selectedFieldId.value = null;
    }
    isDirty.value = true;
  }

  function selectField(id: string | null) {
    selectedFieldId.value = id;
  }

  function toTemplateFields(): SigningTemplateField[] {
    return fields.value.map((f) => ({
      id: f.id,
      name: f.name,
      type: f.type,
      required: f.required,
      pageNumber: f.pageNumber,
      xPercent: f.xPercent,
      yPercent: f.yPercent,
      widthPercent: f.widthPercent,
      heightPercent: f.heightPercent,
      prefillStage: f.prefillStage,
      recipientIndex: f.recipientIndex,
    }));
  }

  function markClean() {
    isDirty.value = false;
  }

  return {
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
    fieldTypeToShort,
  };
}
