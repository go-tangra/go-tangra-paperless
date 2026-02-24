<script lang="ts" setup>
import { ref, watch, nextTick, onMounted } from 'vue';

import { Button, Input } from 'ant-design-vue';

const props = defineProps<{
  modelValue?: string;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: string | undefined];
}>();

const mode = ref<'draw' | 'type'>('draw');
const typedText = ref('');
const canvasRef = ref<HTMLCanvasElement | null>(null);
const isDrawing = ref(false);
let lastX = 0;
let lastY = 0;

function initCanvas() {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  canvas.width = canvas.offsetWidth * (window.devicePixelRatio || 1);
  canvas.height = 150 * (window.devicePixelRatio || 1);
  canvas.style.height = '150px';
  ctx.scale(window.devicePixelRatio || 1, window.devicePixelRatio || 1);
  ctx.strokeStyle = '#1a1a1a';
  ctx.lineWidth = 2;
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';
}

onMounted(() => {
  nextTick(() => initCanvas());
});

function getPosition(e: MouseEvent | TouchEvent): { x: number; y: number } {
  const canvas = canvasRef.value!;
  const rect = canvas.getBoundingClientRect();
  if ('touches' in e) {
    return {
      x: e.touches[0]!.clientX - rect.left,
      y: e.touches[0]!.clientY - rect.top,
    };
  }
  return { x: e.clientX - rect.left, y: e.clientY - rect.top };
}

function startDrawing(e: MouseEvent | TouchEvent) {
  isDrawing.value = true;
  const pos = getPosition(e);
  lastX = pos.x;
  lastY = pos.y;
}

function draw(e: MouseEvent | TouchEvent) {
  if (!isDrawing.value) return;
  e.preventDefault();

  const canvas = canvasRef.value;
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const pos = getPosition(e);
  ctx.beginPath();
  ctx.moveTo(lastX, lastY);
  ctx.lineTo(pos.x, pos.y);
  ctx.stroke();

  lastX = pos.x;
  lastY = pos.y;
}

function stopDrawing() {
  if (!isDrawing.value) return;
  isDrawing.value = false;
  emitImage();
}

function clearCanvas() {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  emit('update:modelValue', undefined);
}

function emitImage() {
  if (mode.value === 'draw') {
    const canvas = canvasRef.value;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
    const hasContent = imageData.data.some(
      (val, i) => i % 4 === 3 && val > 0,
    );
    if (!hasContent) {
      emit('update:modelValue', undefined);
      return;
    }
    const data = canvas.toDataURL('image/png').split(',')[1];
    emit('update:modelValue', data);
  }
}

function renderTypedSignature(): string | undefined {
  if (!typedText.value) return undefined;
  const tempCanvas = document.createElement('canvas');
  tempCanvas.width = 400;
  tempCanvas.height = 100;
  const ctx = tempCanvas.getContext('2d')!;
  ctx.font =
    'italic 36px "Brush Script MT", "Segoe Script", "Dancing Script", cursive';
  ctx.fillStyle = '#1a1a1a';
  ctx.textBaseline = 'middle';
  ctx.fillText(typedText.value, 10, 50);
  return tempCanvas.toDataURL('image/png').split(',')[1];
}

watch(typedText, () => {
  if (mode.value === 'type') {
    emit('update:modelValue', renderTypedSignature());
  }
});

watch(mode, (newMode) => {
  if (newMode === 'draw') {
    nextTick(() => initCanvas());
  } else {
    emit('update:modelValue', renderTypedSignature());
  }
});
</script>

<template>
  <div class="space-y-3">
    <!-- Mode toggle -->
    <div class="flex gap-2">
      <Button
        :type="mode === 'draw' ? 'primary' : 'default'"
        size="small"
        @click="mode = 'draw'"
      >
        Draw
      </Button>
      <Button
        :type="mode === 'type' ? 'primary' : 'default'"
        size="small"
        @click="mode = 'type'"
      >
        Type
      </Button>
    </div>

    <!-- Draw mode -->
    <div v-if="mode === 'draw'">
      <div
        class="rounded border-2 border-dashed border-gray-300 bg-white p-1 dark:border-gray-600 dark:bg-gray-800"
      >
        <canvas
          ref="canvasRef"
          class="w-full cursor-crosshair"
          style="height: 150px"
          @mousedown="startDrawing"
          @mousemove="draw"
          @mouseup="stopDrawing"
          @mouseleave="stopDrawing"
          @touchstart.prevent="startDrawing"
          @touchmove.prevent="draw"
          @touchend="stopDrawing"
        />
      </div>
      <div class="mt-1 flex justify-end">
        <Button size="small" @click="clearCanvas">Clear</Button>
      </div>
    </div>

    <!-- Type mode -->
    <div v-else>
      <Input
        v-model:value="typedText"
        placeholder="Type your full name"
        size="large"
        class="signature-typed-input"
      />
      <div
        v-if="typedText"
        class="mt-2 rounded border bg-white p-3 text-center dark:bg-gray-800"
      >
        <p
          class="text-3xl italic"
          style="
            font-family:
              'Brush Script MT', 'Segoe Script', 'Dancing Script', cursive;
          "
        >
          {{ typedText }}
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.signature-typed-input :deep(input) {
  font-family: 'Brush Script MT', 'Segoe Script', 'Dancing Script', cursive;
  font-size: 24px;
  font-style: italic;
}
</style>
