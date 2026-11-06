<script setup lang="ts">
import { ref, watch } from 'vue'
import { useDialogStore } from '../stores/dialogs'
import ExpandableTextarea from './ExpandableTextarea.vue'

const dialogs = useDialogStore()
const value = ref('')

watch(
  () => dialogs.current,
  (c) => {
    if (c?.kind === 'prompt') value.value = c.value
  },
)

function cancel() {
  const c = dialogs.current
  if (!c) return
  if (c.kind === 'prompt') c.resolve(null)
  else c.resolve(false)
  dialogs.close()
}

function ok() {
  const c = dialogs.current
  if (!c) return
  if (c.kind === 'prompt') {
    if (!value.value.trim()) return
    c.resolve(value.value.trim())
  } else c.resolve(true)
  dialogs.close()
}
</script>

<template>
  <v-dialog :model-value="!!dialogs.current" max-width="440" @update:model-value="(v: boolean) => !v && cancel()">
    <v-card v-if="dialogs.current" :title="dialogs.current.title">
      <v-card-text>
        <template v-if="dialogs.current.kind === 'prompt'">
          <ExpandableTextarea
            v-if="dialogs.current.multiline"
            v-model="value"
            :label="dialogs.current.label"
            rows="4"
            autofocus
          />
          <v-text-field v-else v-model="value" :label="dialogs.current.label" autofocus @keydown.enter="ok" />
        </template>
        <p v-else>{{ dialogs.current.text }}</p>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn @click="cancel">取消</v-btn>
        <v-btn
          :color="dialogs.current.kind === 'confirm' && dialogs.current.danger ? 'error' : 'primary'"
          variant="flat"
          :disabled="dialogs.current.kind === 'prompt' && !value.trim()"
          @click="ok"
        >
          {{ dialogs.current.confirm }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
