import { ref, computed } from 'vue';

import type { SigningField } from '../../../../api/public-client';

export interface FlowField {
  /** Original field from API */
  field: SigningField;
  /** Current value */
  value: string;
  /** Signature image base64 (for signature fields only) */
  signatureImage?: string;
}

export function useSigningFlow() {
  const fields = ref<FlowField[]>([]);
  const activeFieldIndex = ref(0);

  const activeField = computed(() => fields.value[activeFieldIndex.value]);
  const totalFields = computed(() => fields.value.length);
  const progress = computed(() => {
    const filled = fields.value.filter((f) => isFieldFilled(f)).length;
    return { filled, total: totalFields.value };
  });

  function isFieldFilled(f: FlowField): boolean {
    const isSignature =
      f.field.type === 'SIGNING_FIELD_TYPE_SIGNATURE' ||
      f.field.type === 'signature';
    if (isSignature) return !!f.signatureImage;
    return !!f.value?.trim();
  }

  function initializeFields(sessionFields: SigningField[]) {
    const flowFields: FlowField[] = sessionFields.map((field) => ({
      field,
      value: field.prefilledValue || '',
    }));
    // Sort by page then y position
    flowFields.sort((a, b) => {
      if (a.field.pageNumber !== b.field.pageNumber) {
        return a.field.pageNumber - b.field.pageNumber;
      }
      return a.field.yPercent - b.field.yPercent;
    });
    fields.value = flowFields;
    const firstUnfilled = flowFields.findIndex((f) => !isFieldFilled(f));
    activeFieldIndex.value = firstUnfilled >= 0 ? firstUnfilled : 0;
  }

  function goToNext() {
    if (activeFieldIndex.value < fields.value.length - 1) {
      activeFieldIndex.value++;
    }
  }

  function goToPrevious() {
    if (activeFieldIndex.value > 0) {
      activeFieldIndex.value--;
    }
  }

  function goToField(index: number) {
    if (index >= 0 && index < fields.value.length) {
      activeFieldIndex.value = index;
    }
  }

  function setFieldValue(index: number, value: string) {
    const f = fields.value[index];
    if (f) {
      f.value = value;
    }
  }

  function setSignatureImage(index: number, base64: string | undefined) {
    const f = fields.value[index];
    if (f) {
      f.signatureImage = base64;
    }
  }

  const hasPositions = computed(() =>
    fields.value.some((f) => f.field.pageNumber > 0),
  );

  const canSubmit = computed(() => {
    return fields.value.every((f) => {
      if (!f.field.required) return true;
      return isFieldFilled(f);
    });
  });

  const isLastField = computed(
    () => activeFieldIndex.value === fields.value.length - 1,
  );

  function buildSubmitPayload(): {
    fieldValues: Array<{
      fieldId: string;
      value: string;
      pageNumber?: number;
      xPercent?: number;
      yPercent?: number;
      widthPercent?: number;
      heightPercent?: number;
    }>;
    signatureImage?: string;
    signaturePageNumber?: number;
    signatureXPercent?: number;
    signatureYPercent?: number;
    signatureWidthPercent?: number;
    signatureHeightPercent?: number;
  } {
    let signatureImage: string | undefined;
    let signaturePageNumber: number | undefined;
    let signatureXPercent: number | undefined;
    let signatureYPercent: number | undefined;
    let signatureWidthPercent: number | undefined;
    let signatureHeightPercent: number | undefined;

    const fieldValues: Array<{
      fieldId: string;
      value: string;
      pageNumber?: number;
      xPercent?: number;
      yPercent?: number;
      widthPercent?: number;
      heightPercent?: number;
    }> = [];

    for (const f of fields.value) {
      const isSignature =
        f.field.type === 'SIGNING_FIELD_TYPE_SIGNATURE' ||
        f.field.type === 'signature';

      const pos = f.field;

      if (isSignature) {
        signatureImage = f.signatureImage;
        signaturePageNumber = pos.pageNumber;
        signatureXPercent = pos.xPercent;
        signatureYPercent = pos.yPercent;
        signatureWidthPercent = pos.widthPercent;
        signatureHeightPercent = pos.heightPercent;
      } else if (f.value) {
        fieldValues.push({
          fieldId: f.field.fieldId,
          value: f.value,
          pageNumber: pos.pageNumber,
          xPercent: pos.xPercent,
          yPercent: pos.yPercent,
          widthPercent: pos.widthPercent,
          heightPercent: pos.heightPercent,
        });
      }
    }

    return {
      fieldValues,
      signatureImage,
      signaturePageNumber,
      signatureXPercent,
      signatureYPercent,
      signatureWidthPercent,
      signatureHeightPercent,
    };
  }

  return {
    fields,
    activeFieldIndex,
    activeField,
    totalFields,
    progress,
    hasPositions,
    canSubmit,
    isLastField,
    isFieldFilled,
    initializeFields,
    goToNext,
    goToPrevious,
    goToField,
    setFieldValue,
    setSignatureImage,
    buildSubmitPayload,
  };
}
