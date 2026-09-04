<template>
  <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
    <a-collapse-item
      v-for="item in items"
      :key="item.question"
      :header="item.question"
    >
      {{ item.answer }}
    </a-collapse-item>
  </a-collapse>
</template>

<script setup>
defineProps({
  items: { type: Array, default: () => [] },
})
</script>
