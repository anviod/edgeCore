<template>
  <Transition
    name="scale"
    mode="out-in"
    v-bind="$attrs"
  >
    <slot />
  </Transition>
</template>

<script setup>
/**
 * 缩放弹入弹出过渡
 * 适用于弹窗内容、提示卡片、菜单等
 */
</script>
