<script setup lang="ts">
import { mdiFullscreen, mdiFullscreenExit } from '@mdi/js'
import { computed, ref, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })
const props = defineProps<{ label?: string }>()
const value = defineModel<string>({ required: true })
const attrs = useAttrs()
const expanded = ref(false)
const inline = ref<{ $el: HTMLElement; focus: () => void } | null>(null)
const fullscreen = ref<{ $el: HTMLElement } | null>(null)
const label = computed(() => props.label ?? '文本')
let selection = { start: 0, end: 0, direction: 'none' as 'forward' | 'backward' | 'none', scrollTop: 0 }

function fullscreenAttrs() {
  return Object.fromEntries(Object.entries(attrs).filter(([name]) => !name.startsWith('onKey') && ![
    'id', 'class', 'style', 'rows', 'autoGrow', 'auto-grow', 'maxRows', 'max-rows', 'height', 'autofocus',
  ].includes(name)))
}

function remember(element?: HTMLTextAreaElement | null) {
  if (!element) return
  selection = { start: element.selectionStart, end: element.selectionEnd, direction: element.selectionDirection, scrollTop: element.scrollTop }
}

function restore(element?: HTMLTextAreaElement | null) {
  if (!element) return
  element.focus({ preventScroll: true })
  element.setSelectionRange(selection.start, selection.end, selection.direction)
  element.scrollTop = selection.scrollTop
}

function open() {
  remember(inline.value?.$el.querySelector('textarea'))
  expanded.value = true
}

function close() {
  remember(fullscreen.value?.$el.querySelector('textarea'))
  expanded.value = false
}

function onKey(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  event.stopPropagation()
  if (event.isComposing || event.keyCode === 229) return
  event.preventDefault()
  close()
}

defineExpose({ focus: () => inline.value?.focus() })
</script>

<template>
  <div class="sk-expandable-textarea">
    <v-textarea ref="inline" v-bind="attrs" v-model="value" :label="label">
      <template #append-inner>
        <v-tooltip text="全屏编辑" location="top">
          <template #activator="{ props: buttonProps }">
            <v-btn v-bind="buttonProps" :icon="mdiFullscreen" size="small" density="comfortable" :aria-label="`全屏编辑${label}`" @mousedown.prevent @click.stop="open" />
          </template>
        </v-tooltip>
      </template>
    </v-textarea>
    <v-dialog :model-value="expanded" fullscreen :aria-label="`全屏编辑${label}`" @update:model-value="!$event && close()" @after-enter="restore(fullscreen?.$el.querySelector('textarea'))" @after-leave="restore(inline?.$el.querySelector('textarea'))">
      <v-card class="sk-fullscreen-textarea" @keydown.capture="onKey">
        <v-toolbar :title="label">
          <v-spacer />
          <v-btn :icon="mdiFullscreenExit" aria-label="退出全屏编辑" title="退出全屏编辑（Esc）" @click="close" />
        </v-toolbar>
        <v-card-text class="sk-fullscreen-body">
          <v-textarea ref="fullscreen" v-bind="fullscreenAttrs()" v-model="value" :label="label" :rows="1" :auto-grow="false" no-resize class="sk-expanded-textarea" />
        </v-card-text>
      </v-card>
    </v-dialog>
  </div>
</template>

<style scoped>
.sk-expandable-textarea { min-width: 0; }
.sk-fullscreen-textarea { display: flex; flex-direction: column; height: 100%; }
.sk-fullscreen-body { display: flex; flex: 1; min-height: 0; }
.sk-expanded-textarea { flex: 1; min-height: 0; grid-template-rows: minmax(0, 1fr) auto; }
.sk-expanded-textarea :deep(.v-field),
.sk-expanded-textarea :deep(.v-field__field) { height: 100%; min-height: 0; }
</style>
