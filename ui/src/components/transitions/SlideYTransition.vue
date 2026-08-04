<template>
  <Transition
    name="fade-up"
    mode="out-in"
    v-bind="$attrs"
  >
    <slot />
  </Transition>
</template>

<script setup>
/**
 * 纵向滑入滑出过渡（默认向上）
 * 可通过修改 class 或使用自定义 transition name 扩展
 */
</script>
