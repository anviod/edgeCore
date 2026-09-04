<template>
  <TransitionGroup
    :name="name"
    tag="div"
    class="stagger-container"
    v-bind="$attrs"
  >
    <slot />
  </TransitionGroup>
</template>

<script setup>
/**
 * 错开入场动画容器
 * Props:
 *   - name: transition 名称，默认 'list-stagger'
 *   - stagger: 子项可通过 style="--i: index" 自定义延迟
 * 用法：
 *   <StaggerContainer>
 *     <div v-for="(item, i) in list" :key="item.id" :style="{ '--i': i }">...</div>
 *   </StaggerContainer>
 */
defineProps({
  name: {
    type: String,
    default: 'list-stagger'
  }
})
</script>

<style scoped>
.stagger-container {
  display: flex;
  flex-direction: column;
  gap: var(--component-gap, var(--space-3));
}
</style>
