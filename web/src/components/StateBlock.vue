<script setup lang="ts">
// 数据视图的三态：空 / 加载中 / 错误（R-27）。文案由调用处写清原因和下一步。
defineProps<{ state: 'empty' | 'loading' | 'error'; text: string }>()
</script>

<template>
  <div class="sk-state" :role="state === 'error' ? 'alert' : 'status'" :aria-busy="state === 'loading'">
    <v-progress-linear v-if="state === 'loading'" indeterminate color="primary" height="2" class="mb-3" />
    <p class="sk-state-text" :class="{ 'text-error': state === 'error' }">{{ text }}</p>
    <div v-if="$slots.default" class="sk-state-actions"><slot /></div>
  </div>
</template>

<style scoped>
.sk-state {
  padding: 16px 12px;
  font-size: 13px;
  line-height: 1.6;
}
.sk-state-text {
  margin: 0;
  color: var(--sk-muted);
}
.sk-state-text.text-error {
  color: rgb(var(--v-theme-error));
}
.sk-state-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}
</style>
