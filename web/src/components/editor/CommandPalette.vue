<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import type { ParagraphKind } from '../../api/types'
import { hasUnfinishedHalf } from '../../core/command'
import { PARAGRAPH_KINDS, kindLabel } from '../../core/labels'
import { blockedReason, contextLines, degradeNote, type ActionInput } from '../../editor/scaffold'
import type { ArticleSession, CursorInfo } from '../../editor/session'

// 光标即命令面板：在光标处输入自然语言指令，或选一个常用动作。
// 每个动作下方写明会把哪些上下文交给 Scaffolder。

const props = defineProps<{
  session: ArticleSession
  cursor: CursorInfo
  at: { left: number; top: number }
  reason: string
}>()
const emit = defineEmits<{
  (e: 'run', a: ActionInput): void
  (e: 'set-kind', k: ParagraphKind): void
  (e: 'close'): void
}>()

interface Item {
  id: string
  label: string
  hint?: string
  local?: boolean
  disabled?: string | null
  /** 行内可用 ←/→ 切换段落类型 */
  kindPicker?: boolean
  action?: () => ActionInput | null
}

const text = ref('')
const active = ref(0)
const newKind = ref<ParagraphKind>('normal')
const setKind = ref<ParagraphKind>(props.cursor.kind)
const force = ref(false)
const input = ref<HTMLInputElement | null>(null)

const blocked = computed(() => blockedReason(props.session))
const hasSel = computed(() => !!props.cursor.selection && props.cursor.selection.end > props.cursor.selection.start)
const canComplete = computed(() => !hasSel.value && hasUnfinishedHalf(props.cursor.text, props.cursor.cp))

const items = computed<Item[]>(() => {
  const t = text.value.trim()
  const list: Item[] = []
  list.push({
    id: 'custom',
    label: t ? `按指令生成：${t}` : '写下你要 Scaffolder 做什么',
    disabled: t ? null : '先在上面输入指令',
    action: () => ({ mode: 'generate', instruction: t, title: t }),
  })
  if (hasSel.value) {
    list.push(
      { id: 'polish', label: '润色选区', hint: '只改句式，不改意思', action: () => ({ mode: 'polish', instruction: t, title: '润色' }) },
      { id: 'detail', label: '为选区补充细节', action: () => ({ mode: 'detail', instruction: t, title: '补细节' }) },
      { id: 'advise-sel', label: '对选区给建议', hint: '只读建议，不生成正文', action: () => ({ mode: 'advise', instruction: t, title: '建议' }) },
    )
  } else {
    list.push({
      id: 'complete',
      label: '续写光标处的半句',
      disabled: canComplete.value ? null : '光标前没有没写完的半句',
      action: () => ({ mode: 'complete', instruction: '', title: '续写' }),
    })
    list.push(
      { id: 'env', label: '补一句环境描写', action: () => ({ mode: 'generate', instruction: '在光标处补一句环境描写', title: '补环境描写' }) },
      { id: 'act', label: '补人物的动作细节', action: () => ({ mode: 'generate', instruction: '在光标处补人物的动作细节', title: '补动作细节' }) },
      { id: 'psy', label: '补一句心理描写', action: () => ({ mode: 'generate', instruction: '在光标处补一句心理描写', title: '补心理描写' }) },
    )
  }
  list.push({
    id: 'advise',
    label: '给本段建议',
    hint: '只读建议，不生成正文',
    action: () => ({ mode: 'advise', instruction: t || '就这一段给出修改建议', title: '本段建议' }),
  })
  list.push({
    id: 'new',
    label: `在此后新建${kindLabel(newKind.value)}并生成`,
    hint: '←/→ 切换类型',
    kindPicker: true,
    action: () => ({
      mode: 'generate',
      instruction: t || `按上下文写一段${kindLabel(newKind.value)}`,
      newParagraphKind: newKind.value,
      title: `新${kindLabel(newKind.value)}`,
    }),
  })
  list.push({ id: 'kind', label: `把本段设为${kindLabel(setKind.value)}`, hint: '←/→ 切换类型', kindPicker: true, local: true })
  return list.map((it) => (blocked.value && !it.local ? { ...it, disabled: blocked.value } : it))
})

const current = computed(() => items.value[active.value] ?? null)
const currentAction = computed(() => {
  const it = current.value
  if (!it || it.local || !it.action) return null
  const a = it.action()
  return a ? { ...a, force: force.value } : null
})
const preview = computed(() => (currentAction.value ? contextLines(props.session, props.cursor, currentAction.value) : []))
const degraded = computed(() => (currentAction.value ? degradeNote(props.cursor, { ...currentAction.value, force: false }) : null))

const style = computed(() => {
  const w = 460
  const left = Math.max(8, Math.min(props.at.left, window.innerWidth - w - 8))
  const top = Math.min(props.at.top + 6, window.innerHeight - 380)
  return { left: `${left}px`, top: `${Math.max(8, top)}px`, width: `${w}px` }
})

function run(it: Item | null) {
  if (!it || it.disabled) return
  if (it.local) {
    emit('set-kind', setKind.value)
    return
  }
  const a = it.action?.()
  if (a) emit('run', { ...a, force: force.value })
}

function cycleKind(delta: number) {
  const it = current.value
  if (!it?.kindPicker) return false
  const ref_ = it.id === 'new' ? newKind : setKind
  const i = PARAGRAPH_KINDS.findIndex((k) => k.value === ref_.value)
  ref_.value = PARAGRAPH_KINDS[(i + delta + PARAGRAPH_KINDS.length) % PARAGRAPH_KINDS.length]!.value
  return true
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    active.value = (active.value + 1) % items.value.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    active.value = (active.value - 1 + items.value.length) % items.value.length
  } else if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
    if (cycleKind(e.key === 'ArrowLeft' ? -1 : 1)) e.preventDefault()
  } else if (e.key === 'Enter') {
    if ((e.target as HTMLElement).getAttribute('type') === 'checkbox') return
    e.preventDefault()
    run(current.value)
  } else if (e.key === 'Escape') {
    e.preventDefault()
    emit('close')
  } else if (e.key === 'Tab') {
    // 焦点留在面板里：只在输入框与“仍然生成”勾选框之间循环。
    const box = (e.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('input')
    const list = Array.from(box)
    if (list.length === 0) return
    e.preventDefault()
    const i = list.indexOf(document.activeElement as HTMLElement)
    const next = list[(i + (e.shiftKey ? -1 : 1) + list.length) % list.length]
    next?.focus()
  }
}

function onInput() {
  if (text.value.trim()) active.value = 0
}

onMounted(async () => {
  await nextTick()
  input.value?.focus()
  if (!text.value) active.value = items.value.findIndex((i) => !i.disabled)
  if (active.value < 0) active.value = 0
})
</script>

<template>
  <div class="sk-palette-scrim" @mousedown.self="emit('close')">
    <div class="sk-palette bg-surface" :style="style" role="dialog" aria-modal="true" aria-label="命令面板" @keydown="onKey">
      <p class="sk-pal-reason">{{ blocked ?? reason }}</p>
      <input
        ref="input"
        v-model="text"
        class="sk-pal-input"
        type="text"
        :placeholder="hasSel ? '对选区的指令，例如：改成短句，节奏快一点' : '例如：补一段傍晚操场的环境描写'"
        aria-label="给 Scaffolder 的指令"
        role="combobox"
        aria-controls="sk-pal-list"
        :aria-activedescendant="current ? `sk-pal-${current.id}` : undefined"
        aria-expanded="true"
        @input="onInput"
      />
      <ul id="sk-pal-list" class="sk-pal-list" role="listbox" aria-label="动作">
        <li
          v-for="(it, i) in items"
          :id="`sk-pal-${it.id}`"
          :key="it.id"
          role="option"
          class="sk-pal-item"
          :class="{ 'sk-pal-on': i === active, 'sk-pal-off': !!it.disabled }"
          :aria-selected="i === active"
          :aria-disabled="!!it.disabled"
          @mousemove="active = i"
          @mousedown.prevent="run(it)"
        >
          <span class="sk-pal-label">{{ it.label }}</span>
          <span class="sk-pal-hint">{{ it.disabled && i === active ? it.disabled : (it.hint ?? (it.local ? '只改本地，不调用 Scaffolder' : '')) }}</span>
        </li>
      </ul>
      <div v-if="preview.length" class="sk-pal-ctx">
        <p class="sk-pal-ctx-title">会交给 Scaffolder：</p>
        <ul>
          <li v-for="(l, i) in preview" :key="i">{{ l }}</li>
        </ul>
        <div v-if="degraded" class="sk-pal-degrade">
          <p>{{ degraded }}</p>
          <v-checkbox v-model="force" label="仍然生成" density="compact" hide-details />
        </div>
      </div>
      <p class="sk-pal-keys sk-muted">
        <span class="sk-kbd">↑↓</span> 选择 <span class="sk-kbd">Enter</span> 执行 <span class="sk-kbd">Esc</span> 关闭
      </p>
    </div>
  </div>
</template>

<style scoped>
.sk-palette-scrim {
  position: fixed;
  inset: 0;
  z-index: 2000;
}
.sk-palette {
  position: fixed;
  border: 1px solid var(--sk-line);
  border-radius: 4px;
  box-shadow: 0 6px 24px rgba(var(--v-theme-on-surface), 0.18);
  padding: 8px;
  font-size: 13px;
  max-height: calc(100vh - 16px);
  overflow: auto;
}
.sk-pal-reason {
  margin: 0 0 6px;
  color: var(--sk-muted);
  line-height: 1.5;
}
.sk-pal-input {
  width: 100%;
  border: 1px solid var(--sk-line);
  border-radius: 3px;
  padding: 6px 8px;
  font: inherit;
  font-size: 14px;
  color: rgb(var(--v-theme-on-surface));
  background: none;
}
.sk-pal-input:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
  outline-offset: -1px;
}
.sk-pal-list {
  list-style: none;
  margin: 6px 0;
  padding: 0;
}
.sk-pal-item {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 5px 8px;
  border-radius: 3px;
  cursor: pointer;
}
.sk-pal-on {
  background-color: rgba(var(--v-theme-primary), 0.14);
  box-shadow: inset 2px 0 0 rgb(var(--v-theme-primary));
}
.sk-pal-off .sk-pal-label {
  color: var(--sk-muted);
}
.sk-pal-label {
  flex: 1;
}
.sk-pal-hint {
  font-size: 12px;
  color: var(--sk-muted);
}
.sk-pal-ctx {
  border-top: 1px solid var(--sk-line);
  padding-top: 6px;
  line-height: 1.6;
}
.sk-pal-ctx-title {
  margin: 0;
  font-size: 12px;
  color: var(--sk-muted);
}
.sk-pal-ctx ul {
  margin: 2px 0 0;
  padding-left: 18px;
}
.sk-pal-degrade {
  margin-top: 6px;
  padding: 6px 8px;
  border: 1px solid rgba(var(--v-theme-warning), 0.7);
  border-radius: 3px;
}
.sk-pal-degrade p {
  margin: 0;
}
.sk-pal-keys {
  margin: 6px 0 0;
  font-size: 12px;
}
</style>
