import type { Node as PMNode } from 'prosemirror-model'
import { Decoration, DecorationSet } from 'prosemirror-view'
import type { AnalysisMark, ParagraphAnalysis } from '../api/types'
import { findCommands } from '../core/command'
import { cpToU16 } from '../core/offsets'
import { halfAt, segment } from '../core/textseg'
import { COLORED_DESC } from '../core/theme'
import type { ArgMarkSpec } from './nodeview'

// 所有图层都是 Decoration：文本本身不带任何格式，分析失效时图层直接消失，正文不受影响。

export interface Layers {
  desc: boolean
  marks: boolean
  logic: boolean
  diag: boolean
}

export interface WidgetSpec {
  id: string
  key: string
  cp: number
  /** 流式生成中 / 已生成待评审 */
  phase: 'streaming' | 'pending'
  title: string
  text: string
  /** 新段落类建议以块的形式显示在段落下方 */
  block: boolean
  onReview?: () => void
  onCancel?: () => void
}

export interface TargetSpec {
  key: string
  start: number
  end: number
  focused: boolean
}

export interface DecoContext {
  layers: Layers
  analysisFor(key: string, text: string): ParagraphAnalysis | null
  hover: { key: string; cp: number } | null
  cursor: { key: string; cp: number } | null
  widgets: WidgetSpec[]
  targets: TargetSpec[]
  drop: { key: string; side: 'before' | 'after' } | null
}

const ARG_KINDS = new Set(['claim', 'evidence', 'reasoning'])

function primaryDesc(desc: string[]): string | null {
  for (const d of desc) if ((COLORED_DESC as string[]).includes(d)) return d
  return null
}

function markClass(m: AnalysisMark): string | null {
  switch (m.kind) {
    case 'theme':
      return 'sk-mk-theme'
    case 'keyword':
      return 'sk-mk-keyword'
    case 'repetition':
      return 'sk-mk-repeat'
    case 'pronoun':
      return 'sk-mk-pronoun'
    case 'callback':
      return 'sk-mk-callback'
    case 'claim':
    case 'evidence':
    case 'reasoning':
      return `sk-mk-arg sk-mk-${m.kind}`
  }
}

function widgetDOM(w: WidgetSpec): HTMLElement {
  const box = document.createElement(w.block ? 'div' : 'span')
  box.className = `sk-sg-widget sk-sg-${w.phase}${w.block ? ' sk-sg-block' : ''}`
  box.contentEditable = 'false'
  box.setAttribute('role', 'note')
  box.setAttribute('aria-label', `Scaffolder 建议：${w.title}，尚未写入正文`)

  const head = document.createElement('span')
  head.className = 'sk-sg-head'
  head.textContent = w.phase === 'streaming' ? `${w.title} · 生成中，Esc 取消` : `${w.title} · 待评审`
  box.append(head)

  const body = document.createElement('span')
  body.className = 'sk-sg-text'
  body.textContent = w.text || (w.phase === 'streaming' ? '等待第一段文字' : '')
  box.append(body)

  const actions = document.createElement('span')
  actions.className = 'sk-sg-actions'
  if (w.phase === 'pending' && w.onReview) {
    const b = document.createElement('button')
    b.type = 'button'
    b.textContent = '逐半句评审'
    b.addEventListener('mousedown', (e) => e.preventDefault())
    b.addEventListener('click', () => w.onReview?.())
    actions.append(b)
  }
  if (w.phase === 'streaming' && w.onCancel) {
    const b = document.createElement('button')
    b.type = 'button'
    b.textContent = '取消'
    b.addEventListener('mousedown', (e) => e.preventDefault())
    b.addEventListener('click', () => w.onCancel?.())
    actions.append(b)
  }
  if (actions.childNodes.length) box.append(actions)
  return box
}

function paragraphDecorations(node: PMNode, pos: number, ctx: DecoContext, out: Decoration[]) {
  const key = node.attrs.key as string
  const text = node.textContent
  const base = pos + 1
  const at = (cp: number) => base + cpToU16(text, cp)
  const len = text.length

  // 段落类型样式与拖放指示
  const nodeClasses: string[] = []
  if (ctx.drop?.key === key) nodeClasses.push(`sk-drop-${ctx.drop.side}`)
  if (len === 0 && ctx.cursor?.key === key) nodeClasses.push('sk-ph')

  // [ ... ] 命令
  for (const c of findCommands(text)) {
    out.push(Decoration.inline(at(c.start), at(c.end), { class: 'sk-cmd', title: `Scaffolder 指令：${c.instruction}。Shift+Enter 执行，不算正文` }, { inclusiveStart: false, inclusiveEnd: false }))
  }

  // 半句边界：光标所在、悬停所在
  const seg = segment(text)
  const cur = ctx.cursor?.key === key ? halfAt(seg, ctx.cursor.cp) : null
  const hov = ctx.hover?.key === key ? halfAt(seg, ctx.hover.cp) : null
  for (const h of seg.halves) {
    const cls: string[] = ['sk-half']
    if (cur && cur.index === h.index) cls.push('sk-half-cur')
    if (hov && hov.index === h.index) cls.push('sk-half-hover')
    out.push(Decoration.inline(at(h.start), at(h.end), { class: cls.join(' '), 'data-half': String(h.index) }))
  }

  const a = ctx.analysisFor(key, text)
  if (a) {
    if (ctx.layers.desc) {
      for (const h of a.halves) {
        const range = a.segmentation.halves[h.index]
        const d = primaryDesc(h.desc)
        if (!range || !d) continue
        out.push(Decoration.inline(at(range.start), at(range.end), { class: `sk-desc sk-desc-${d}` }))
      }
    }
    if (ctx.layers.marks) {
      const argMarks: ArgMarkSpec['argMarks'] = []
      const hoverCp = ctx.hover?.key === key ? ctx.hover.cp : -1
      for (const m of a.marks) {
        const cls = markClass(m)
        if (cls && m.end > m.start) out.push(Decoration.inline(at(m.start), at(m.end), { class: cls }))
        if (ARG_KINDS.has(m.kind)) argMarks.push({ kind: m.kind as 'claim', note: m.note })
        if (m.kind === 'pronoun' && m.ref && hoverCp >= m.start && hoverCp < m.end) {
          out.push(Decoration.inline(at(m.ref.start), at(m.ref.end), { class: 'sk-ref-target' }))
          out.push(Decoration.inline(at(m.start), at(m.end), { class: 'sk-ref-source' }))
        }
      }
      if (argMarks.length) out.push(Decoration.node(pos, pos + node.nodeSize, {}, { argMarks }))
    }
    if (ctx.layers.diag) {
      for (const d of a.diagnostics) {
        if (d.end <= d.start) continue
        out.push(Decoration.inline(at(d.start), at(d.end), { class: `sk-diag sk-diag-${d.severity}` }))
      }
    }
  }

  for (const t of ctx.targets) {
    if (t.key !== key || t.end <= t.start) continue
    out.push(Decoration.inline(at(t.start), at(t.end), { class: t.focused ? 'sk-sg-target sk-sg-target-focus' : 'sk-sg-target' }))
  }

  for (const w of ctx.widgets) {
    if (w.key !== key) continue
    const p = w.block ? base + len : at(w.cp)
    out.push(
      Decoration.widget(p, () => widgetDOM(w), {
        side: 1,
        ignoreSelection: true,
        key: `${w.id}:${w.phase}:${w.text.length}`,
        stopEvent: () => true,
      }),
    )
  }

  if (nodeClasses.length) out.push(Decoration.node(pos, pos + node.nodeSize, { class: nodeClasses.join(' ') }))
}

export function buildDecorations(doc: PMNode, ctx: DecoContext): DecorationSet {
  const out: Decoration[] = []
  doc.forEach((node, offset) => paragraphDecorations(node, offset, ctx, out))
  return DecorationSet.create(doc, out)
}
