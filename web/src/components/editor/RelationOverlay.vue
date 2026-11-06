<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { relationLabel } from '../../core/labels'
import { cpToU16 } from '../../core/offsets'
import { halfAt, segment } from '../../core/textseg'
import type { ArticleSession } from '../../editor/session'

// 半句之间的逻辑关系：SVG 弧线箭头，从语序靠前的 from 指向 to，标注关系名（SPEC §6）。
// 默认只画悬停（或光标）所在半句的关系；“逻辑”图层打开时画全部。

const props = defineProps<{ session: ArticleSession; paper: HTMLElement | null }>()

interface Arc {
  id: string
  d: string
  lx: number
  ly: number
  label: string
  note?: string
  focus: boolean
}

const tick = ref(0)
let ro: ResizeObserver | null = null

onMounted(() => {
  ro = new ResizeObserver(() => tick.value++)
  if (props.paper) ro.observe(props.paper)
})
onBeforeUnmount(() => ro?.disconnect())

function anchor(view: NonNullable<ArticleSession['view']>, base: number, text: string, s: number, e: number, origin: DOMRect) {
  try {
    const a = view.coordsAtPos(base + cpToU16(text, s), 1)
    const b = view.coordsAtPos(base + cpToU16(text, e), -1)
    const sameLine = Math.abs(a.top - b.top) < 4
    const x = sameLine ? (a.left + b.right) / 2 : a.left + 16
    return { x: x - origin.left, y: a.top - origin.top + 2 }
  } catch {
    return null
  }
}

const arcs = computed<Arc[]>(() => {
  void tick.value
  void props.session.ui.revision
  const view = props.session.view
  const paper = props.paper
  if (!view || !paper) return []
  const all = props.session.ui.layers.logic
  const focusPoint = props.session.ui.hover ?? (props.session.ui.cursor ? { key: props.session.ui.cursor.key, cp: props.session.ui.cursor.cp } : null)
  const origin = paper.getBoundingClientRect()
  const out: Arc[] = []
  props.session.doc.forEach((node, pos) => {
    const key = node.attrs.key as string
    const text = node.textContent
    if (!all && focusPoint?.key !== key) return
    const a = props.session.validAnalysis(key, text)
    if (!a || a.relations.length === 0) return
    const focusHalf = focusPoint?.key === key ? halfAt(segment(text), focusPoint.cp)?.index : undefined
    for (const [i, r] of a.relations.entries()) {
      const touches = r.from === focusHalf || r.to === focusHalf
      if (!all && !touches) continue
      const hf = a.segmentation.halves[r.from]
      const ht = a.segmentation.halves[r.to]
      if (!hf || !ht) continue
      const p = anchor(view, pos + 1, text, hf.start, hf.end, origin)
      const q = anchor(view, pos + 1, text, ht.start, ht.end, origin)
      if (!p || !q) continue
      const dx = Math.abs(q.x - p.x)
      const h = Math.min(46, 14 + dx * 0.18 + Math.abs(q.y - p.y) * 0.1)
      const top = Math.min(p.y, q.y) - h
      out.push({
        id: `${key}-${i}`,
        d: `M ${p.x} ${p.y} C ${p.x} ${top}, ${q.x} ${top}, ${q.x} ${q.y}`,
        lx: (p.x + q.x) / 2,
        ly: top + h * 0.25 - 2,
        label: relationLabel(r.kind),
        note: r.note,
        focus: touches,
      })
    }
  })
  return out
})
</script>

<template>
  <svg v-if="arcs.length" class="sk-rel" aria-hidden="true">
    <defs>
      <marker id="sk-rel-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
        <path d="M 0 0 L 8 4 L 0 8 z" class="sk-rel-head" />
      </marker>
    </defs>
    <g v-for="a in arcs" :key="a.id" :class="{ 'sk-rel-focus': a.focus }">
      <path :d="a.d" class="sk-rel-path" marker-end="url(#sk-rel-arrow)"><title>{{ a.note ?? a.label }}</title></path>
      <text :x="a.lx" :y="a.ly" text-anchor="middle" class="sk-rel-label">{{ a.label }}</text>
    </g>
  </svg>
</template>

<style scoped>
.sk-rel {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  overflow: visible;
  z-index: 1;
}
.sk-rel-path {
  fill: none;
  stroke: rgba(var(--v-theme-primary), 0.55);
  stroke-width: 1.25;
}
.sk-rel-focus .sk-rel-path {
  stroke: rgb(var(--v-theme-primary));
  stroke-width: 1.5;
}
.sk-rel-head {
  fill: rgb(var(--v-theme-primary));
}
.sk-rel-label {
  font-family: var(--sk-font-ui);
  font-size: 11px;
  fill: rgb(var(--v-theme-on-surface));
  paint-order: stroke;
  stroke: rgb(var(--v-theme-sk-editor));
  stroke-width: 4px;
}
</style>
