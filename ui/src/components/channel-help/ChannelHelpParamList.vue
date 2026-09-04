<template>
  <div class="help-doc-card">
    <div v-for="item in items" :key="item.label" class="help-doc-row" :class="{ 'help-doc-row--stack': !item.example }">
      <div class="help-doc-row__label">
        <strong>{{ item.label }}</strong>
        <span v-if="item.hint">{{ item.hint }}</span>
        <span v-if="item.desc && !item.example" class="help-doc-row__desc">{{ item.desc }}</span>
      </div>
      <code v-if="item.example" class="help-doc-row__value">{{ item.example }}</code>
    </div>
  </div>
</template>

<script setup>
defineProps({
  items: { type: Array, default: () => [] },
})
</script>
