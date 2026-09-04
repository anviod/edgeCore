<template>
  <div class="relative flex h-2 w-2">
    <!-- 外层脉冲环 (仅运行和故障状态显示) -->
    <span
      v-if="showPulse"
      class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
      :class="pulseColorClass"
    ></span>
    <!-- 内层实心点 -->
    <span
      class="relative inline-flex h-2 w-2 rounded-full"
      :class="dotColorClass"
    ></span>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  status: { type: String, default: 'unknown' },
})

const showPulse = computed(() => props.status === 'running' || props.status === 'fault')

const dotColorClass = computed(() => {
  switch (props.status) {
    case 'running': return 'bg-emerald-500'
    case 'standby': return 'bg-amber-500'
    case 'fault': return 'bg-red-500'
    default: return 'bg-slate-400'
  }
})

const pulseColorClass = computed(() => {
  switch (props.status) {
    case 'running': return 'bg-emerald-400'
    case 'fault': return 'bg-red-400'
    default: return ''
  }
})
</script>

