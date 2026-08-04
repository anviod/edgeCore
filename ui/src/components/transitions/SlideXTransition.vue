<template>
  <Transition
    name="fade-left"
    mode="out-in"
    v-bind="$attrs"
  >
    <slot />
  </Transition>
</template>

<script setup>
/**
 * 横向滑入滑出过渡（默认向左退出，用于前进导航）
 * 如需返回效果请使用 name="fade-right" 或配合路由方向判断
 */
</script>
