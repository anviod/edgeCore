<template>
  <Transition
    name="fade"
    mode="out-in"
    v-bind="$attrs"
  >
    <slot />
  </Transition>
</template>

<script setup>
/**
 * 全局淡入淡出过渡组件
 * 用法：<FadeTransition><component /></FadeTransition>
 */
</script>
