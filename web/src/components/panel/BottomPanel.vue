<script setup lang="ts">
import { mdiChevronDown, mdiDeleteSweepOutline } from '@mdi/js'
import { computed, ref, watch } from 'vue'
import type { Diagnostic } from '../../api/types'
import { sliceCp } from '../../core/offsets'
import { halfAt, segment } from '../../core/textseg'
import { useEditorStore } from '../../stores/editor'
import { useLogStore } from '../../stores/log'
import { useUiStore } from '../../stores/ui'
import StateBlock from '../StateBlock.vue'

const ui = useUiStore()
const editor = useEditorStore()
const log = useLogStore()
const outEl = ref<HTMLElement | null>(null)

interface Problem {
  id: string
  key: string
  paragraph: number
  half: number | null
  excerpt: string
  d: Diagnostic
}

const SEVERITY: Record<Diagnostic['severity'], string> = { hint: '提示', info: '信息', warning: '注意' }

const problems = computed<Problem[]>(() => {
  const s = editor.activeSession
  if (!s) return []
  void s.ui.revision
  const out: Problem[] = []
  let idx = 0
  s.doc.forEach((node) => {
    idx++
    const key = node.attrs.key as string
    const text = node.textContent
    const a = s.validAnalysis(key, text)
    if (!a) return
    const seg = segment(text)
    a.diagnostics.forEach((d, i) => {
      out.push({
        id: `${key}-${i}`,
        key,
        paragraph: idx,
        half: halfAt(seg, d.start)?.index ?? null,
        excerpt: sliceCp(text, d.start, Math.min(d.end, d.start + 16)),
        d,
      })
    })
  })
  return out
})

const analyzedAny = computed(() => {
  const s = editor.activeSession
  if (!s) return false
  void s.ui.revision
  let any = false
  s.doc.forEach((n) => {
    if (s.validAnalysis(n.attrs.key as string, n.textContent)) any = true
  })
  return any
})

function go(p: Problem) {
  editor.activeSession?.reveal(p.key, p.d.start, p.d.end)
}

watch(
  () => [log.entries.at(-1)?.seq, ui.panelTab, ui.layout.panelOpen, ui.layout.panelHeight, outEl.value],
  () => {
    if (ui.panelTab !== 'output' || !ui.layout.panelOpen) return
    const el = outEl.value
    if (el) el.scrollTop = el.scrollHeight
  },
  { flush: 'post', immediate: true },
)
</script>

<template>
  <section class="sk-bottom bg-sk-panel" aria-label="底部面板">
    <div class="sk-bottom-tabs">
      <v-tabs v-model="ui.panelTab" density="comfortable" color="primary" aria-label="底部面板视图">
      <v-tab value="problems">
        问题<span v-if="problems.length" class="sk-count">{{ problems.length }}</span>
      </v-tab>
      <v-tab value="output">
        输出
      </v-tab>
      </v-tabs>
      <span class="sk-fill" />
      <v-btn
        v-if="ui.panelTab === 'output'"
        :icon="mdiDeleteSweepOutline"
        size="x-small"
        density="comfortable"
        aria-label="清空输出"
        title="清空输出"
        @click="log.clear()"
      />
      <v-btn :icon="mdiChevronDown" size="x-small" density="comfortable" aria-label="收起底部面板（Ctrl+J）" title="收起（Ctrl+J）" @click="ui.layout.panelOpen = false" />
    </div>

    <div v-if="ui.panelTab === 'problems'" class="sk-bottom-body" role="tabpanel" aria-label="问题">
      <StateBlock v-if="!editor.activeSession" state="empty" text="打开一篇文章后，这里列出 Analyzer 指出的问题。" />
      <StateBlock
        v-else-if="!analyzedAny"
        state="empty"
        text="这篇还没有有效的分析结果。点编辑器工具条上的“分析全文”，或在设置里打开自动分析。"
      />
      <StateBlock v-else-if="problems.length === 0" state="empty" text="已分析的段落里，Analyzer 没有指出问题。" />
      <ul v-else class="sk-problems">
        <li v-for="p in problems" :key="p.id">
          <button type="button" class="sk-problem" @click="go(p)">
            <span class="sk-sev" :class="`sk-sev-${p.d.severity}`">{{ SEVERITY[p.d.severity] }}</span>
            <span class="sk-problem-msg">{{ p.d.message }}</span>
            <span class="sk-problem-loc">第 {{ p.paragraph }} 段{{ p.half !== null ? ` · 第 ${p.half + 1} 半句` : '' }} · “{{ p.excerpt }}”</span>
          </button>
        </li>
      </ul>
    </div>

    <div v-else ref="outEl" class="sk-bottom-body sk-output" role="tabpanel" aria-label="输出">
      <StateBlock v-if="log.entries.length === 0" state="empty" text="还没有 Agent 请求。分析、生成、取消都会在这里留一行记录。" />
      <ol v-else class="sk-log">
        <li v-for="e in log.entries" :key="e.seq" :class="`sk-log-${e.dir}`">
          <span class="sk-log-time">{{ e.time }}</span>
          <span class="sk-log-dir">{{ e.dir === 'out' ? '发出' : e.dir === 'in' ? '收到' : '连接' }}</span>
          <span class="sk-log-text">{{ e.text }}</span>
          <span v-if="e.detail" class="sk-log-detail">{{ e.detail }}</span>
        </li>
      </ol>
    </div>
  </section>
</template>

<style scoped>
.sk-bottom {
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-top: 1px solid var(--sk-line);
}
.sk-bottom-tabs {
  flex: none;
  display: flex;
  align-items: center;
  min-height: 48px;
  padding: 0 4px 0 8px;
}
.sk-btab {
  height: 32px;
  padding: 0 10px;
  font: inherit;
  font-size: 12px;
  background: none;
  border: 0;
  color: var(--sk-muted);
  cursor: pointer;
  box-shadow: inset 0 -1px 0 rgba(var(--v-theme-primary), 0);
}
.sk-btab-on {
  color: rgb(var(--v-theme-on-surface));
  box-shadow: inset 0 -2px 0 rgb(var(--v-theme-primary));
}
.sk-count {
  margin-left: 6px;
  font-size: 11px;
}
.sk-fill {
  flex: 1;
}
.sk-bottom-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.sk-problems {
  list-style: none;
  margin: 0;
  padding: 0 0 8px;
}
.sk-problem {
  display: flex;
  align-items: baseline;
  gap: 10px;
  width: 100%;
  padding: 3px 12px;
  background: none;
  border: 0;
  color: inherit;
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.sk-problem:hover {
  background-color: rgba(var(--v-theme-on-surface), var(--v-hover-opacity));
}
.sk-problem:focus-visible {
  outline-offset: -2px;
}
.sk-sev {
  flex: none;
  font-size: 11px;
  padding: 0 4px;
  border: 1px solid var(--sk-line);
  border-radius: 2px;
}
.sk-sev-warning {
  border-color: rgb(var(--v-theme-warning));
}
.sk-sev-info {
  border-color: rgb(var(--v-theme-info));
}
.sk-problem-msg {
  flex: 1;
}
.sk-problem-loc {
  flex: none;
  font-size: 12px;
  color: var(--sk-muted);
}
.sk-log {
  list-style: none;
  margin: 0;
  padding: 4px 0 8px;
  font-family: var(--sk-font-code);
  font-size: 12px;
  line-height: 1.6;
}
.sk-log li {
  padding: 0 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 0 10px;
}
.sk-log-time,
.sk-log-dir {
  color: var(--sk-muted);
}
.sk-log-detail {
  flex-basis: 100%;
  color: var(--sk-muted);
  white-space: pre-wrap;
  word-break: break-all;
  padding-left: 2em;
}
</style>
