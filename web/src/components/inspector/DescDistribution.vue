<script setup lang="ts">
import { computed } from 'vue'
import type { DescKind } from '../../api/types'
import { DESC_KINDS, descLabel, kindShort } from '../../core/labels'
import { cpLength } from '../../core/offsets'
import { COLORED_DESC } from '../../core/theme'
import type { ArticleSession } from '../../editor/session'
import { useAnalysisStore } from '../../stores/analysis'

// 描写分布：每段一条，按半句长度分段着色。只把 Analyzer 的识别结果摆出来，不打分、不下结论。

const props = defineProps<{ session: ArticleSession }>()
const analysis = useAnalysisStore()

interface Seg {
  half: number
  start: number
  end: number
  width: number
  desc: DescKind | null
  all: DescKind[]
}

interface Row {
  key: string
  index: number
  kind: string
  status: 'ok' | 'stale' | 'none' | 'empty'
  segs: Seg[]
  counts: Partial<Record<DescKind, number>>
}

const rows = computed<Row[]>(() => {
  void props.session.ui.revision
  const out: Row[] = []
  props.session.doc.forEach((node, _pos, index) => {
    const key = node.attrs.key as string
    const text = node.textContent
    const row: Row = { key, index, kind: node.attrs.kind as string, status: 'none', segs: [], counts: {} }
    if (text.trim() === '') row.status = 'empty'
    else {
      const a = props.session.validAnalysis(key, text)
      if (!a) row.status = props.session.hasStaleAnalysis(key) ? 'stale' : 'none'
      else {
        row.status = 'ok'
        const total = Math.max(1, cpLength(text))
        for (const h of a.halves) {
          const r = a.segmentation.halves[h.index]
          if (!r) continue
          const desc = (h.desc.find((d) => (COLORED_DESC as string[]).includes(d)) ?? null) as DescKind | null
          row.segs.push({ half: h.index, start: r.start, end: r.end, width: ((r.end - r.start) / total) * 100, desc, all: h.desc })
          for (const d of h.desc) row.counts[d] = (row.counts[d] ?? 0) + 1
        }
      }
    }
    out.push(row)
  })
  return out
})

const totals = computed(() => {
  const t: Partial<Record<DescKind, number>> = {}
  for (const r of rows.value) for (const [k, v] of Object.entries(r.counts)) t[k as DescKind] = (t[k as DescKind] ?? 0) + (v ?? 0)
  return DESC_KINDS.map((d) => ({ ...d, n: t[d.value] ?? 0 }))
})
const analyzed = computed(() => rows.value.filter((r) => r.status === 'ok').length)
const nonEmpty = computed(() => rows.value.filter((r) => r.status !== 'empty').length)
const busy = computed(() => analysis.st(props.session.articleId).inflight > 0)

function go(row: Row, seg?: Seg) {
  props.session.reveal(row.key, seg ? seg.start : 0, seg ? seg.end : 0)
}

function segTitle(seg: Seg) {
  return `第 ${seg.half + 1} 半句：${seg.all.length ? seg.all.map(descLabel).join('、') : '无描写类型'}`
}
</script>

<template>
  <div class="sk-dist">
    <p class="sk-dist-sum sk-muted">
      已分析 {{ analyzed }} / {{ nonEmpty }} 段
      <button v-if="analyzed < nonEmpty" type="button" class="sk-plain text-decoration-underline ml-1" :disabled="busy" @click="analysis.analyzeParagraphs(session, rows.filter((r) => r.status === 'none' || r.status === 'stale').map((r) => r.key))">
        {{ busy ? '分析中' : '分析其余段落' }}
      </button>
    </p>

    <ol class="sk-dist-rows" aria-label="每段的描写分布">
      <li v-for="r in rows" :key="r.key" class="sk-dist-row">
        <button type="button" class="sk-dist-label" :title="`跳到第 ${r.index + 1} 段`" @click="go(r)">
          {{ r.index + 1 }}<span class="sk-muted">〔{{ kindShort(r.kind) }}〕</span>
        </button>
        <div v-if="r.status === 'ok'" class="sk-dist-bar" role="group" :aria-label="`第 ${r.index + 1} 段`">
          <button
            v-for="seg in r.segs"
            :key="seg.half"
            type="button"
            class="sk-dist-seg"
            :class="seg.desc ? `sk-desc-${seg.desc}` : 'sk-dist-none'"
            :style="{ flexBasis: `${seg.width}%`, backgroundColor: seg.desc ? `rgb(var(--v-theme-desc-${seg.desc}))` : undefined }"
            :title="segTitle(seg)"
            :aria-label="segTitle(seg)"
            @click="go(r, seg)"
          />
        </div>
        <span v-else class="sk-dist-state sk-muted">
          {{ r.status === 'empty' ? '空段' : r.status === 'stale' ? '改动后待重新分析' : '未分析' }}
        </span>
      </li>
    </ol>

    <div class="sk-dist-legend">
      <p class="sk-muted text-caption mb-1">全文半句数（一个半句可以同时属于几类）</p>
      <ul>
        <li v-for="t in totals" :key="t.value">
          <span class="sk-dist-chip" :class="t.colored ? `sk-desc-${t.value}` : 'sk-dist-none'" :style="t.colored ? { backgroundColor: `rgb(var(--v-theme-desc-${t.value}))` } : undefined" aria-hidden="true" />
          <span>{{ t.title }}</span>
          <span class="sk-dist-n">{{ t.n }}</span>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.sk-dist {
  padding: 8px 12px 16px;
  font-size: 13px;
}
.sk-dist-sum {
  margin: 0 0 8px;
  font-size: 12px;
}
.sk-dist-rows {
  list-style: none;
  margin: 0;
  padding: 0;
}
.sk-dist-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 24px;
}
.sk-dist-label {
  flex: 0 0 64px;
  text-align: left;
  font: inherit;
  font-size: 12px;
  font-family: var(--sk-font-body);
  background: none;
  border: 0;
  color: inherit;
  cursor: pointer;
  padding: 0;
}
.sk-dist-bar {
  flex: 1;
  display: flex;
  height: 14px;
  border: 1px solid var(--sk-line);
  border-radius: 2px;
  overflow: hidden;
}
.sk-dist-seg {
  min-width: 3px;
  border: 0;
  border-right: 1px solid rgb(var(--v-theme-sk-panel));
  padding: 0;
  cursor: pointer;
}
.sk-dist-seg:last-child {
  border-right: 0;
}
.sk-dist-seg:focus-visible {
  outline-offset: -2px;
}
.sk-dist-none {
  background-color: rgba(var(--v-theme-on-surface), 0.06);
}
.sk-dist-state {
  font-size: 12px;
}
.sk-dist-legend {
  margin-top: 16px;
  border-top: 1px solid var(--sk-line);
  padding-top: 8px;
}
.sk-dist-legend ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 2px 12px;
}
.sk-dist-legend li {
  display: flex;
  align-items: center;
  gap: 6px;
}
.sk-dist-chip {
  width: 12px;
  height: 12px;
  border: 1px solid var(--sk-line);
  border-radius: 2px;
}
.sk-dist-n {
  margin-left: auto;
  font-variant-numeric: tabular-nums;
}
</style>
