<script setup lang="ts">
import { mdiClose, mdiMagnify } from '@mdi/js'
import { computed, ref, watch } from 'vue'
import type { ParagraphKind } from '../../api/types'
import { hasUnfinishedHalf } from '../../core/command'
import { PARAGRAPH_KINDS, kindLabel } from '../../core/labels'
import { blockedReason, contextLines, degradeNote, type ActionInput } from '../../editor/scaffold'
import type { ArticleSession, CursorInfo } from '../../editor/session'

export type EditorUtility = 'find' | 'replace' | 'fullscreen' | 'organize' | 'analyze-current' | 'analyze-all' | 'settings'
const props = defineProps<{ session: ArticleSession; cursor: CursorInfo; reason: string; busy: boolean }>()
const emit = defineEmits<{ run: [action: ActionInput]; 'set-kind': [kind: ParagraphKind]; utility: [action: EditorUtility]; close: [] }>()

interface Item {
  id: string
  label: string
  hint?: string
  disabled?: string
  action?: ActionInput
  utility?: EditorUtility
  kind?: boolean
}

const text = ref('')
const active = ref(0)
const expandedId = ref<string | null>(null)
const newKind = ref<ParagraphKind>('normal')
const setKind = ref<ParagraphKind>(props.cursor.kind)
const force = ref(false)
const blocked = computed(() => blockedReason(props.session))
const hasSelection = computed(() => !!props.cursor.selection)

const items = computed<Item[]>(() => {
  const instruction = text.value.trim()
  const utilities: Item[] = [
    { id: 'organize', label: 'AI 重新整理全文段落类型', hint: '按全文推断每段类型，预览后统一应用', utility: 'organize', disabled: props.busy ? '正在分析' : undefined },
    { id: 'find', label: '查找', hint: 'Ctrl+F', utility: 'find' },
    { id: 'replace', label: '查找并替换', hint: 'Ctrl+H', utility: 'replace' },
    { id: 'fullscreen', label: '切换全屏', hint: 'F11', utility: 'fullscreen' },
    { id: 'analyze-current', label: '分析本段', utility: 'analyze-current', disabled: props.busy ? '正在分析' : undefined },
    { id: 'analyze-all', label: '分析全文', utility: 'analyze-all', disabled: props.busy ? '正在分析' : undefined },
    { id: 'settings', label: '打开设置', hint: 'Ctrl+,', utility: 'settings' },
  ]
  const writing: Item[] = [
    { id: 'custom', label: instruction ? `按指令生成：${instruction}` : '输入写作指令', action: { mode: 'generate', instruction, title: instruction }, disabled: instruction ? undefined : '在上方输入指令' },
    ...(hasSelection.value ? [
      { id: 'polish', label: '润色选区', action: { mode: 'polish' as const, instruction, title: '润色' } },
      { id: 'detail', label: '为选区补充细节', action: { mode: 'detail' as const, instruction, title: '补细节' } },
    ] : [
      { id: 'complete', label: '续写光标处的半句', disabled: hasUnfinishedHalf(props.cursor.text, props.cursor.cp) ? undefined : '光标前没有未完成的半句', action: { mode: 'complete' as const, instruction: '', title: '续写' } },
      { id: 'environment', label: '补一句环境描写', action: { mode: 'generate' as const, instruction: instruction || '在光标处补一句环境描写', title: '补环境描写' } },
      { id: 'action', label: '补人物的动作细节', action: { mode: 'generate' as const, instruction: instruction || '在光标处补人物的动作细节', title: '补动作细节' } },
      { id: 'psychology', label: '补一句心理描写', action: { mode: 'generate' as const, instruction: instruction || '在光标处补一句心理描写', title: '补心理描写' } },
    ]),
    { id: 'advice', label: hasSelection.value ? '对选区给建议' : '给本段建议', hint: '只读建议，不生成正文', action: { mode: 'advise', instruction: instruction || '就当前内容给出修改建议', title: '建议' } },
    { id: 'new', label: `在此后新建${kindLabel(newKind.value)}并生成`, action: { mode: 'generate', instruction: instruction || `按上下文写一段${kindLabel(newKind.value)}`, newParagraphKind: newKind.value, title: `新${kindLabel(newKind.value)}` } },
    { id: 'kind', label: `把本段设为${kindLabel(setKind.value)}`, kind: true, disabled: props.session.ui.kindLocked ? '类型推断中，取消后台任务后可手动切换' : undefined },
  ]
  const filtered = instruction ? utilities.filter((item) => `${item.label} ${item.hint ?? ''}`.toLowerCase().includes(instruction.toLowerCase())) : utilities
  return [...filtered, ...writing.map((item) => item.action && blocked.value ? { ...item, disabled: blocked.value } : item)]
})

const current = computed(() => items.value[active.value])
const expanded = computed(() => items.value.find((item) => item.id === expandedId.value))
const preview = computed(() => expanded.value?.action ? contextLines(props.session, props.cursor, expanded.value.action) : [])
const degraded = computed(() => expanded.value?.action ? degradeNote(props.cursor, expanded.value.action) : null)
watch(text, () => { active.value = 0; expandedId.value = null })

function activate(item?: Item) {
  if (!item || item.disabled) return
  active.value = items.value.findIndex((candidate) => candidate.id === item.id)
  if (item.kind || item.action) {
    expandedId.value = item.id
    force.value = false
  } else run(item)
}

function run(item?: Item) {
  if (!item || item.disabled) return
  if (item.utility) emit('utility', item.utility)
  else if (item.kind) emit('set-kind', setKind.value)
  else if (item.action) emit('run', { ...item.action, force: force.value })
}

function onKey(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  if (event.target instanceof Element && !event.target.closest('input[role="combobox"]')) return
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    active.value = (active.value + (event.key === 'ArrowDown' ? 1 : -1) + items.value.length) % items.value.length
    document.getElementById(`sk-command-${current.value?.id}`)?.scrollIntoView({ block: 'nearest' })
  } else if (event.key === 'Enter') {
    event.preventDefault()
    if (expanded.value?.id === current.value?.id) run(expanded.value)
    else activate(current.value)
  }
}
</script>

<template>
  <v-dialog :model-value="true" max-width="640" scrollable aria-label="命令面板" @update:model-value="!$event && emit('close')">
    <v-card title="命令面板" @keydown="onKey">
      <template #append><v-btn :icon="mdiClose" aria-label="关闭命令面板" @click="emit('close')" /></template>
      <v-card-text class="pb-2">
        <v-text-field v-model="text" label="搜索命令或输入写作指令" :prepend-inner-icon="mdiMagnify" autofocus role="combobox" aria-controls="sk-command-list" :aria-activedescendant="current ? `sk-command-${current.id}` : undefined" aria-expanded="true" hide-details />
        <p class="text-body-2 sk-muted mt-3 mb-0">{{ reason }}</p>
      </v-card-text>
      <v-list id="sk-command-list" role="listbox" aria-label="可用命令" density="compact" max-height="360" class="overflow-y-auto">
        <v-list-item v-for="(item, index) in items" :id="`sk-command-${item.id}`" :key="item.id" role="option" :aria-selected="active === index" :active="active === index" :disabled="!!item.disabled" :title="item.label" :subtitle="item.disabled || item.hint" @focus="active = index" @click="activate(item)" />
      </v-list>
      <v-card-text v-if="expanded" class="pt-3 pb-2">
        <v-select v-if="expanded.kind" v-model="setKind" :items="PARAGRAPH_KINDS" item-value="value" item-title="title" label="段落类型" />
        <v-select v-if="expanded.id === 'new'" v-model="newKind" :items="PARAGRAPH_KINDS" item-value="value" item-title="title" label="新段落类型" />
        <v-expansion-panels v-if="preview.length" variant="accordion" class="mt-2">
          <v-expansion-panel title="生成时使用的上下文"><v-expansion-panel-text><ul class="pl-4 text-body-2"><li v-for="line in preview" :key="line">{{ line }}</li></ul></v-expansion-panel-text></v-expansion-panel>
        </v-expansion-panels>
        <v-alert v-if="degraded" type="info" variant="tonal" class="mt-3" density="compact">{{ degraded }}<v-checkbox v-model="force" label="仍然生成" hide-details /></v-alert>
      </v-card-text>
      <v-card-actions><span class="text-caption sk-muted pl-4">↑↓ 选择 · Enter 打开 · Esc 关闭</span><v-spacer /><v-btn color="primary" :disabled="!(expanded || current) || !!(expanded || current)?.disabled" @click="expanded ? run(expanded) : activate(current)">{{ expanded ? '执行' : '打开' }}</v-btn></v-card-actions>
    </v-card>
  </v-dialog>
</template>
