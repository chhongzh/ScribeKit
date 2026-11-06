<script setup lang="ts">
defineProps<{ title: string }>()
</script>

<template>
  <header class="sk-panel-header">
    <h2 class="sk-panel-title">{{ title }}</h2>
    <div class="sk-panel-actions"><slot /></div>
  </header>
</template>

<style scoped>
.sk-panel-header {
  flex: none;
  display: flex;
  align-items: center;
  height: 36px;
  padding: 0 4px 0 12px;
  border-bottom: 1px solid var(--sk-line);
}
.sk-panel-title {
  flex: 1;
  font-size: 12px;
  font-weight: 600;
  margin: 0;
  color: var(--sk-muted);
}
.sk-panel-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}
</style>
