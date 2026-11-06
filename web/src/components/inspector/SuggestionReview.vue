<script setup lang="ts">
import { mdiArrowLeft } from '@mdi/js'
import { computed, nextTick, ref } from 'vue'
import type { Suggestion, SuggestionHalf } from '../../api/types'
import { descLabel, kindLabel, rhetoricLabel } from '../../core/labels'
import { canApply, finalText, undecidedCount } from '../../core/review'
import { COLORED_DESC } from '../../core/theme'
import type { ArticleSession } from '../../editor/session'
import { useDialogStore } from '../../stores/dialogs'
import { useSuggestionStore } from '../../stores/suggestions'
import StateBlock from '../StateBlock.vue'

// 建议评审：AI 生成整句，用户逐半句决定接受、拒绝或就地改写。全部决定后才能合入。

const props = defineProps<{ suggestion: Suggestion; session: ArticleSession }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const sg = useSuggestionStore()
const dialogs = useDialogStore()
const s = props.suggestion

const editing = ref<number | null>(null)
const draft = ref('')
const list = ref<HTMLElement | null>(null)

const ta = computed(() => sg.textAnalysis[s.id])
const degraded = computed(() => sg.degraded[s.id])
const busy = computed(() => sg.busy[s.id])
const remaining = computed(() => undecidedCount(s.halves))
const ready = computed(() => canApply(s.halves))
const result = computed(() => finalText(s.halves))

const where = computed(() => {
  const t = s.target
  if (s.op === 'advice' || !t) return '只读建议：不会写入正文'
  if ('paragraphId' in t) {
    const key = props.session.sync.keyOf(t.paragraphId)
    const idx = key ? props.session.find(key)?.index : undefined
    return idx !== undefined ? `替换第 ${idx + 1} 段第 ${t.start}–${t.end} 字` : '替换的目标段落已不在文中'
  }
  if (t.afterParagraphId === null) return `作为新的${kindLabel(s.paragraphKind)}放在开头`
  const key = props.session.sync.keyOf(t.afterParagraphId)
  const idx = key ? props.session.find(key)?.index : undefined
  return idx !== undefined ? `作为新的${kindLabel(s.paragraphKind)}放在第 ${idx + 1} 段之后` : `作为新的${kindLabel(s.paragraphKind)}插入`
})

function descOf(h: SuggestionHalf) {
  const a = ta.value?.data
  if (!a) return null
  return a.halves.find((x) => x.index === h.index) ?? null
}

function bgClass(h: SuggestionHalf) {
  const d = descOf(h)?.desc.find((k) => (COLORED_DESC as string[]).includes(k))
  return d ? `sk-desc-${d}` : ''
}

function stateLabel(h: SuggestionHalf) {
  switch (h.state) {
    case 'accepted':
      return '已接受'
    case 'rejected':
      return '已拒绝'
    case 'edited':
      return '已改写'
    default:
      return '未决定'
  }
}

async function startEdit(h: SuggestionHalf) {
  editing.value = h.index
  draft.value = h.state === 'edited' ? (h.edited ?? h.text) : h.text
  await nextTick()
  const el = list.value?.querySelector<HTMLTextAreaElement>(`#sk-rv-edit-${h.index}`)
  el?.focus()
  el?.select()
}

function saveEdit(h: SuggestionHalf) {
  if (draft.value === h.text) sg.decide(s, h.index, 'accepted')
  else sg.decide(s, h.index, 'edited', draft.value)
  editing.value = null
  focusRow(h.index + 1)
}

function cancelEdit(h: SuggestionHalf) {
  editing.value = null
  focusRow(h.index)
}

function decide(h: SuggestionHalf, state: 'accepted' | 'rejected') {
  sg.decide(s, h.index, state)
  focusRow(h.index + 1)
}

function focusRow(i: number) {
  void nextTick(() => {
    const rows = list.value?.querySelectorAll<HTMLElement>('.sk-rv-row')
    const el = rows?.[Math.min(i, (rows?.length ?? 1) - 1)]
    el?.focus()
  })
}

function onRowKey(e: KeyboardEvent, h: SuggestionHalf, i: number) {
  if (editing.value !== null) return
  if (e.key === 'a' || e.key === 'y') {
    e.preventDefault()
    decide(h, 'accepted')
  } else if (e.key === 'r' || e.key === 'n') {
    e.preventDefault()
    decide(h, 'rejected')
  } else if (e.key === 'e' || e.key === 'F2') {
    e.preventDefault()
    void startEdit(h)
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    focusRow(i + 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    focusRow(Math.max(0, i - 1))
  }
}

async function apply() {
  const warn = sg.applyWarning(s)
  if (warn) {
    const ok = await dialogs.confirm({ title: '确认合入', text: `${warn}仍然合入吗？`, confirm: '仍然合入' })
    if (!ok) return
  }
  await sg.apply(s)
}

async function rejectAll() {
  await sg.reject(s)
}

function locate() {
  const t = s.target
  if (!t) return
  if ('paragraphId' in t) props.session.revealId(t.paragraphId, t.start, t.end)
  else if (t.afterParagraphId !== null) {
    const key = props.session.sync.keyOf(t.afterParagraphId)
    const text = key ? props.session.textOfKey(key) : null
    if (key && text !== null) props.session.reveal(key, Array.from(text).length)
  }
}
</script>

<template>
  <div class="sk-review">
    <div class="sk-rv-head">
      <v-btn :icon="mdiArrowLeft" size="x-small" density="comfortable" aria-label="返回建议列表" @click="emit('close')" />
      <div class="flex-1-1">
        <p class="sk-rv-title">{{ s.instruction || '建议' }}</p>
        <button v-if="s.target" type="button" class="sk-plain sk-muted text-caption text-decoration-underline" @click="locate">{{ where }}</button>
        <p v-else class="sk-muted text-caption mb-0">{{ where }}</p>
      </div>
    </div>

    <div v-if="degraded" class="sk-rv-degrade" role="note">
      <p>这是{{ kindLabel(s.paragraphKind) }}，按产品原则由你亲自写，所以 Scaffolder 只给了建议。确实需要它写正文，可以强制生成。</p>
      <v-btn size="small" variant="outlined" @click="sg.force(s)">仍然生成</v-btn>
    </div>

    <template v-if="s.op === 'advice'">
      <div class="sk-rv-advice">
        <p class="sk-rv-advice-text">{{ s.rationale || s.text }}</p>
        <p v-if="s.rationale && s.text && s.text !== s.rationale" class="sk-rv-advice-text sk-muted">{{ s.text }}</p>
      </div>
      <div class="sk-rv-foot">
        <v-btn size="small" :loading="busy === '正在拒绝'" @click="rejectAll">看过了，移除</v-btn>
      </div>
    </template>

    <template v-else>
      <p v-if="s.rationale" class="sk-rv-why">{{ s.rationale }}</p>

      <StateBlock v-if="ta?.status === 'loading'" state="loading" text="Analyzer 正在分析这条建议，高亮稍后出现。可以先开始评审。" />
      <div v-else-if="ta?.status === 'error'" class="sk-rv-note text-error" role="alert">
        建议文本没有分析成功：{{ ta.error }}
        <button type="button" class="sk-plain text-decoration-underline" @click="sg.analyzeSuggestion(s)">重试</button>
      </div>

      <p class="sk-rv-keys sk-muted">逐个半句决定：<span class="sk-kbd">A</span> 接受 <span class="sk-kbd">R</span> 拒绝 <span class="sk-kbd">E</span> 改写 <span class="sk-kbd">↑↓</span> 换一句</p>

      <ol ref="list" class="sk-rv-list" aria-label="建议的半句">
        <li
          v-for="(h, i) in s.halves"
          :key="h.index"
          class="sk-rv-row"
          :class="`sk-rv-${h.state}`"
          tabindex="0"
          :aria-label="`第 ${i + 1} 个半句，${stateLabel(h)}：${h.state === 'edited' ? h.edited : h.text}`"
          @keydown="onRowKey($event, h, i)"
        >
          <div v-if="editing === h.index" class="sk-rv-edit">
            <textarea
              :id="`sk-rv-edit-${h.index}`"
              v-model="draft"
              class="sk-rv-textarea"
              rows="2"
              aria-label="改写这个半句"
              @keydown.enter.exact.prevent="saveEdit(h)"
              @keydown.esc.prevent.stop="cancelEdit(h)"
            />
            <div class="d-flex ga-1 mt-1">
              <v-btn size="x-small" color="primary" variant="flat" @click="saveEdit(h)">保存改写（Enter）</v-btn>
              <v-btn size="x-small" @click="cancelEdit(h)">取消（Esc）</v-btn>
            </div>
          </div>
          <template v-else>
            <p class="sk-review-half sk-rv-text">
              <span :class="bgClass(h)" :style="h.state === 'rejected' ? 'text-decoration: line-through' : ''">{{ h.text }}</span>
              <span v-if="h.state === 'edited'" class="sk-rv-edited">改为：{{ h.edited }}</span>
            </p>
            <p v-if="descOf(h)" class="sk-rv-meta sk-muted">
              {{ descOf(h)!.desc.map(descLabel).join('、') || '无描写类型' }}<template v-if="descOf(h)!.rhetoric.length"> · {{ descOf(h)!.rhetoric.map(rhetoricLabel).join('、') }}</template>
            </p>
            <div class="sk-rv-actions">
              <span class="sk-rv-state">{{ stateLabel(h) }}</span>
              <v-btn size="x-small" :variant="h.state === 'accepted' ? 'flat' : 'outlined'" :color="h.state === 'accepted' ? 'primary' : undefined" tabindex="-1" @click="decide(h, 'accepted')">接受</v-btn>
              <v-btn size="x-small" :variant="h.state === 'rejected' ? 'flat' : 'outlined'" tabindex="-1" @click="decide(h, 'rejected')">拒绝</v-btn>
              <v-btn size="x-small" :variant="h.state === 'edited' ? 'flat' : 'outlined'" tabindex="-1" @click="startEdit(h)">改写</v-btn>
            </div>
          </template>
        </li>
      </ol>

      <div class="sk-rv-foot">
        <p class="sk-rv-result-label sk-muted">合入后的文字</p>
        <p class="sk-rv-result">{{ result || '（全部拒绝时没有可合入的文字）' }}</p>
        <p class="sk-muted text-caption" role="status">
          <template v-if="remaining">还有 {{ remaining }} 个半句没决定，决定完才能合入。</template>
          <template v-else-if="!ready">没有接受或改写任何半句。不需要的话整条拒绝。</template>
          <template v-else>全部半句已决定。</template>
        </p>
        <div class="d-flex ga-2 flex-wrap">
          <v-btn color="primary" variant="flat" size="small" :disabled="!ready || !!busy" :loading="busy === '正在合入'" @click="apply">合入正文</v-btn>
          <v-btn size="small" :disabled="!!busy" :loading="busy === '正在拒绝'" @click="rejectAll">整条拒绝</v-btn>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.sk-review {
  padding-bottom: 16px;
}
.sk-rv-head {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 8px 12px 8px 6px;
  border-bottom: 1px solid var(--sk-line);
}
.sk-rv-title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}
.sk-rv-degrade,
.sk-rv-note {
  margin: 8px 12px;
  padding: 8px 10px;
  font-size: 13px;
  border: 1px solid rgba(var(--v-theme-warning), 0.7);
  border-radius: 3px;
}
.sk-rv-degrade p {
  margin: 0 0 6px;
}
.sk-rv-why {
  margin: 8px 12px;
  font-size: 13px;
  color: var(--sk-muted);
}
.sk-rv-keys {
  margin: 8px 12px 4px;
  font-size: 12px;
}
.sk-rv-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.sk-rv-row {
  padding: 8px 12px;
  border-bottom: 1px solid var(--sk-line);
}
.sk-rv-row:focus-visible {
  outline-offset: -2px;
}
.sk-rv-pending {
  box-shadow: inset 2px 0 0 rgba(var(--v-theme-primary), 0.6);
}
.sk-rv-rejected .sk-rv-text {
  color: var(--sk-muted);
}
.sk-rv-text {
  margin: 0;
}
.sk-rv-edited {
  display: block;
  font-size: 15px;
  margin-top: 2px;
}
.sk-rv-meta {
  margin: 2px 0 0;
  font-size: 12px;
}
.sk-rv-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 6px;
}
.sk-rv-state {
  flex: 1;
  font-size: 12px;
  color: var(--sk-muted);
}
.sk-rv-textarea {
  width: 100%;
  font-family: var(--sk-font-body);
  font-size: 15px;
  line-height: 1.7;
  padding: 4px 6px;
  border: 1px solid rgb(var(--v-theme-primary));
  border-radius: 3px;
  color: rgb(var(--v-theme-on-surface));
  background: none;
  resize: vertical;
}
.sk-rv-foot {
  padding: 10px 12px;
}
.sk-rv-result-label {
  margin: 0;
  font-size: 12px;
}
.sk-rv-result {
  margin: 2px 0 6px;
  font-family: var(--sk-font-body);
  font-size: 15px;
  line-height: 1.8;
  white-space: pre-wrap;
}
.sk-rv-advice {
  padding: 8px 12px;
}
.sk-rv-advice-text {
  margin: 0 0 8px;
  font-size: 14px;
  line-height: 1.8;
  white-space: pre-wrap;
}
</style>
