<template>
  <div class="border border-slate-200 bg-white p-4">
    <div class="flex items-center justify-between">
      <span class="text-xs font-medium uppercase tracking-wide text-slate-500">{{ label }}</span>
      <StatusIndicator v-if="showStatus" :status="status" />
    </div>
    <div class="mt-2 flex items-baseline gap-1">
      <span class="text-2xl font-semibold text-slate-900">{{ value }}</span>
      <span class="text-sm text-slate-500">{{ unit }}</span>
    </div>
    <div v-if="trend" class="mt-2 flex items-center gap-1">
      <svg
        v-if="trend > 0"
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        class="h-3 w-3 text-emerald-500"
      >
        <polyline points="18 15 12 9 6 15"></polyline>
      </svg>
      <svg
        v-else
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        class="h-3 w-3 text-red-500"
      >
        <polyline points="6 9 12 15 18 9"></polyline>
      </svg>
      <span class="text-xs" :class="trend > 0 ? 'text-emerald-600' : 'text-red-600'">
        {{ Math.abs(trend) }}%
      </span>
      <span class="text-xs text-slate-400">较昨日</span>
    </div>
  </div>
</template>

<script setup>
import StatusIndicator from './StatusIndicator.vue'

withDefaults(defineProps({
  label: { type: String, required: true },
  value: { type: [String, Number], required: true },
  unit: { type: String, required: true },
  trend: { type: Number, default: 0 },
  status: { type: String, default: 'unknown' },
  showStatus: { type: Boolean, default: false }
}), {
  trend: 0,
  status: 'unknown',
  showStatus: false
})
</script>

