<template>
  <span
    class="inline-flex items-center rounded-[2px] px-1.5 py-0.5 font-mono text-[10px] font-medium leading-none text-white"
    :class="bgColorClass"
  >
    {{ displayName }}
  </span>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  protocol: { type: String, default: 'modbus' },
})

const protocolMap = {
  modbus: { name: 'Modbus/TCP', color: 'bg-sky-500' },
  'modbus-rtu': { name: 'Modbus/RTU', color: 'bg-sky-600' },
  opcua: { name: 'OPC UA', color: 'bg-indigo-500' },
  mqtt: { name: 'MQTT', color: 'bg-emerald-500' },
  iec104: { name: 'IEC 104', color: 'bg-purple-500' },
  bacnet: { name: 'BACnet', color: 'bg-amber-600' },
}

const config = computed(() => protocolMap[props.protocol] || { name: props.protocol.toUpperCase(), color: 'bg-slate-500' })
const displayName = computed(() => config.value.name)
const bgColorClass = computed(() => config.value.color)
</script>

