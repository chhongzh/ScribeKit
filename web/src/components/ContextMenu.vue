<script setup lang="ts">
import { ref } from 'vue'

export interface ContextAction {
  title: string
  hint?: string
  disabled?: boolean
  danger?: boolean
  run: () => void
}

defineProps<{ at: [number, number]; label: string; actions: ContextAction[] }>()
const open = defineModel<boolean>({ default: false })
const list = ref<{ $el: HTMLElement } | null>(null)
const emit = defineEmits<{ closed: [] }>()

function focusFirst() {
  list.value?.$el.querySelector<HTMLElement>('.v-list-item:not(.v-list-item--disabled)')?.focus()
}
</script>

<template>
  <v-menu v-model="open" :target="at" location="bottom start" :max-width="'min(360px, calc(100vw - 16px))'" @after-enter="focusFirst" @after-leave="emit('closed')">
    <v-list ref="list" density="compact" role="menu" :aria-label="label">
      <v-list-item v-for="action in actions" :key="action.title" role="menuitem" :title="action.title" :disabled="action.disabled" :base-color="action.danger ? 'error' : undefined" @click="action.run">
        <template v-if="action.hint" #append><span class="sk-kbd ml-4">{{ action.hint }}</span></template>
      </v-list-item>
    </v-list>
  </v-menu>
</template>
