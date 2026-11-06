<script setup lang="ts">
import { computed } from 'vue'
import { useNotifyStore } from '../stores/notify'

const notify = useNotifyStore()
const current = computed(() => notify.queue[0] ?? null)

function close() {
  notify.shift()
}

function run() {
  current.value?.action?.run()
  close()
}
</script>

<template>
  <v-snackbar
    :key="current?.id"
    :model-value="!!current"
    :timeout="current?.timeout ?? 5000"
    location="bottom right"
    :color="current?.tone === 'error' ? 'error' : undefined"
    role="status"
    @update:model-value="(v: boolean) => !v && close()"
  >
    {{ current?.text }}
    <template #actions>
      <v-btn v-if="current?.action" variant="outlined" @click="run">{{ current.action.label }}</v-btn>
      <v-btn aria-label="关闭提示" @click="close">关闭</v-btn>
    </template>
  </v-snackbar>
</template>
