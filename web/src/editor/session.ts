import { baseKeymap } from 'prosemirror-commands'
import { history, redo, undo } from 'prosemirror-history'
import { keymap } from 'prosemirror-keymap'
import type { Node as PMNode } from 'prosemirror-model'
import { EditorState, Plugin, PluginKey, TextSelection, type Transaction } from 'prosemirror-state'
import { DecorationSet, EditorView } from 'prosemirror-view'
import { reactive, shallowReactive } from 'vue'
import type { Article, ArticleDetail, Paragraph, ParagraphKind, ParagraphAnalysis } from '../api/types'
import { textHash } from '../core/hash'
import { stripCommands } from '../core/command'
import { countChars, cpToU16, u16ToCp } from '../core/offsets'
import { halfAt, segment } from '../core/textseg'
import { enterCommand, insertParagraphAfter, moveParagraph, paragraphIndexAt } from './commands'
import { buildDecorations, type DecoContext, type Layers, type TargetSpec, type WidgetSpec } from './decorations'
import { ParagraphView } from './nodeview'
import type { LocalRow, ServerRow } from './plan'
import { keyForId, makeParagraph, newKey, schema } from './schema'
import { SyncEngine } from './sync'

// 一篇打开的文章：ProseMirror 状态 + 同步引擎 + 编辑器 UI 状态。
// 标签页切走时 session 保留（撤销历史也保留），关闭标签页才销毁。

export interface CursorInfo {
  key: string
  id: number | null
  index: number
  kind: ParagraphKind
  cp: number
  text: string
  sentence: number | null
  half: number | null
  selection: { key: string; start: number; end: number } | null
  crossParagraph: boolean
}

export interface SessionCallbacks {
  onKindMenu(key: string, anchor: HTMLElement): void
  onNewParagraphMenu(key: string): void
  onPalette(): void
  onShiftEnter(): void
  onEscape(): boolean
  onTextEdited(keys: string[]): void
}

const decoKey = new PluginKey<DecorationSet>('sk-deco')
const REFRESH = 'sk-refresh'
const REMOTE = 'sk-remote'

/** 粘贴、复制等可能带进重复或空 key，这里统一补成唯一 key。 */
const keyNormalizer = new Plugin({
  appendTransaction(trs, _old, state) {
    if (!trs.some((t) => t.docChanged)) return null
    const seen = new Set<string>()
    let tr: Transaction | null = null
    state.doc.forEach((node, offset) => {
      const k = node.attrs.key as string
      if (!k || seen.has(k)) {
        tr = (tr ?? state.tr).setNodeMarkup(offset, undefined, { ...node.attrs, key: newKey() })
      } else seen.add(k)
    })
    return tr
  },
})

function docFrom(paragraphs: Paragraph[]): PMNode {
  const nodes = paragraphs.length
    ? paragraphs.map((p) => makeParagraph(p.kind, p.text, keyForId(p.id)))
    : [makeParagraph('normal')]
  return schema.nodes.doc!.create(null, nodes)
}

function rowsFrom(paragraphs: Paragraph[]): ServerRow[] {
  return paragraphs.map((p) => ({ key: keyForId(p.id), id: p.id, kind: p.kind, text: p.text, position: p.position }))
}

export class ArticleSession {
  readonly articleId: number
  readonly meta: Article
  sync: SyncEngine
  view: EditorView | null = null
  private state: EditorState
  private cb: SessionCallbacks | null = null

  /** 编辑器外部数据（分析、建议）由 store 注入。 */
  analysisLookup: (id: number) => ParagraphAnalysis | undefined = () => undefined
  widgets: WidgetSpec[] = []
  targets: TargetSpec[] = []

  readonly ui = reactive({
    layers: { desc: true, marks: true, logic: false, diag: true } as Layers,
    cursor: null as CursorInfo | null,
    charCount: 0,
    paragraphCount: 0,
    hover: null as { key: string; cp: number } | null,
    drop: null as { key: string; side: 'before' | 'after' } | null,
    /** 每次文档或装饰变化 +1，供 Vue 侧（关系弧线、问题列表、描写分布）重算。 */
    revision: 0,
  })
  private dragKey: string | null = null

  constructor(detail: ArticleDetail) {
    this.articleId = detail.id
    const { paragraphs, ...meta } = detail
    this.meta = shallowReactive({ ...meta }) as Article
    // 先建同步引擎：装饰插件初始化时就要按 key 查服务器 id。
    this.sync = new SyncEngine(rowsFrom(paragraphs), detail.id, () => this.localRows())
    this.state = this.createState(paragraphs)
    this.updateCounts()
  }

  private createState(paragraphs: Paragraph[]): EditorState {
    return EditorState.create({
      doc: docFrom(paragraphs),
      plugins: [
        history(),
        keymap({
          Enter: enterCommand,
          'Shift-Enter': () => {
            this.cb?.onShiftEnter()
            return true
          },
          // 段内换行（诗歌分行）。Shift+Enter 留给 Scaffolder，所以用 Ctrl/Cmd+Enter。
          'Mod-Enter': (state, dispatch) => {
            dispatch?.(state.tr.replaceSelectionWith(schema.nodes.hard_break!.create()).scrollIntoView())
            return true
          },
          'Alt-Enter': (state) => {
            const key = state.selection.$from.parent.attrs.key as string
            this.cb?.onNewParagraphMenu(key)
            return true
          },
          'Mod-k': () => {
            this.cb?.onPalette()
            return true
          },
          'Ctrl-Space': () => {
            this.cb?.onPalette()
            return true
          },
          Escape: () => this.cb?.onEscape() ?? false,
          'Alt-t': (state) => {
            const key = state.selection.$head.parent.attrs.key as string
            const f = this.find(key)
            const dom = f && this.view ? (this.view.nodeDOM(f.pos) as HTMLElement | null) : null
            const btn = dom?.querySelector<HTMLElement>('.sk-kind')
            if (btn) this.cb?.onKindMenu(key, btn)
            return true
          },
          'Alt-Shift-ArrowUp': () => this.moveCurrent(-1),
          'Alt-Shift-ArrowDown': () => this.moveCurrent(1),
          'Mod-z': undo,
          'Mod-y': redo,
          'Shift-Mod-z': redo,
        }),
        keymap(baseKeymap),
        keyNormalizer,
        new Plugin<DecorationSet>({
          key: decoKey,
          state: {
            init: (_c, s) => buildDecorations(s.doc, this.decoContext(s)),
            apply: (tr, old, _o, s) => {
              // 输入法组字期间不重建装饰，只做位置映射，否则会打断中文输入。
              if (this.view?.composing && !tr.getMeta(REFRESH)) return old.map(tr.mapping, tr.doc)
              return buildDecorations(s.doc, this.decoContext(s))
            },
          },
          props: {
            decorations: (s) => decoKey.getState(s),
          },
        }),
      ],
    })
  }

  // ---------- 读取 ----------

  get doc(): PMNode {
    return this.view?.state.doc ?? this.state.doc
  }

  localRows(): LocalRow[] {
    const rows: LocalRow[] = []
    this.doc.forEach((n) => rows.push({ key: n.attrs.key as string, kind: n.attrs.kind as ParagraphKind, text: n.textContent }))
    return rows
  }

  /** 段落 key → {节点, 起始位置, 下标} */
  find(key: string): { node: PMNode; pos: number; index: number } | null {
    let found: { node: PMNode; pos: number; index: number } | null = null
    this.doc.forEach((node, pos, index) => {
      if (!found && node.attrs.key === key) found = { node, pos, index }
    })
    return found
  }

  textOfKey(key: string): string | null {
    return this.find(key)?.node.textContent ?? null
  }

  textOfId(id: number): string | null {
    const key = this.sync.keyOf(id)
    return key ? this.textOfKey(key) : null
  }

  /** 当前文本对应的有效分析；textHash 对不上就视为没有。 */
  validAnalysis(key: string, text: string): ParagraphAnalysis | null {
    const id = this.sync.idOf(key)
    if (id === null) return null
    const a = this.analysisLookup(id)
    if (!a || a.textHash !== textHash(text)) return null
    return a
  }

  hasStaleAnalysis(key: string): boolean {
    const id = this.sync.idOf(key)
    if (id === null) return false
    const a = this.analysisLookup(id)
    const text = this.textOfKey(key)
    return !!a && text !== null && a.textHash !== textHash(text)
  }

  private decoContext(s: EditorState): DecoContext {
    const sel = s.selection
    const parent = sel.$head.parent
    const cursor = { key: parent.attrs.key as string, cp: u16ToCp(parent.textContent, sel.$head.parentOffset) }
    return {
      layers: this.ui.layers,
      analysisFor: (key, text) => this.validAnalysis(key, text),
      hover: this.ui.hover,
      cursor,
      widgets: this.widgets,
      targets: this.targets,
      drop: this.ui.drop,
    }
  }

  // ---------- 挂载 ----------

  mount(el: HTMLElement, cb: SessionCallbacks): EditorView {
    this.cb = cb
    if (this.view) {
      el.appendChild(this.view.dom)
      return this.view
    }
    const view = new EditorView(el, {
      state: this.state,
      attributes: {
        class: 'sk-doc',
        spellcheck: 'false',
        'aria-label': `正文：${this.meta.title || '未命名'}`,
        'aria-multiline': 'true',
        role: 'textbox',
      },
      nodeViews: {
        paragraph: (node, v) =>
          new ParagraphView(node, v, {
            onKindClick: (key, anchor) => this.cb?.onKindMenu(key, anchor),
            onDragStart: (key, ev) => this.startDrag(key, ev),
            onDragEnd: () => this.endDrag(),
          }),
      },
      dispatchTransaction: (tr) => this.dispatch(tr),
      handleDOMEvents: {
        dragover: (_v, ev) => this.dragOver(ev),
        drop: (_v, ev) => this.dropOn(ev),
        mousemove: (_v, ev) => {
          this.trackHover(ev)
          return false
        },
        mouseleave: () => {
          this.setHover(null)
          return false
        },
      },
    })
    this.view = view
    this.updateCursor()
    return view
  }

  destroy(): void {
    this.view?.destroy()
    this.view = null
    this.cb = null
  }

  private dispatch(tr: Transaction): void {
    const view = this.view
    if (!view) return
    const before = view.state
    const next = before.apply(tr)
    view.updateState(next)
    this.state = next
    if (tr.docChanged) {
      this.updateCounts()
      if (!tr.getMeta(REMOTE)) {
        this.sync.notify()
        this.cb?.onTextEdited(changedKeys(before.doc, next.doc))
      }
    }
    this.updateCursor()
    this.ui.revision++
  }

  /** 外部数据变化后重建装饰。 */
  refresh(): void {
    const view = this.view
    if (view) view.dispatch(view.state.tr.setMeta(REFRESH, true))
    else this.state = this.state.apply(this.state.tr.setMeta(REFRESH, true))
  }

  private updateCounts(): void {
    let n = 0
    this.doc.forEach((p) => (n += countChars(stripCommands(p.textContent))))
    this.ui.charCount = n
    this.ui.paragraphCount = this.doc.childCount
  }

  private updateCursor(): void {
    const s = this.view?.state ?? this.state
    const sel = s.selection
    const $head = sel.$head
    const para = $head.parent
    const key = para.attrs.key as string
    const text = para.textContent
    const cp = u16ToCp(text, $head.parentOffset)
    const seg = segment(text)
    const h = halfAt(seg, cp)
    const $from = sel.$from
    const $to = sel.$to
    const crossParagraph = !sel.empty && $from.parent !== $to.parent
    let selection: CursorInfo['selection'] = null
    if (!sel.empty && !crossParagraph) {
      const t = $from.parent.textContent
      selection = {
        key: $from.parent.attrs.key as string,
        start: u16ToCp(t, $from.parentOffset),
        end: u16ToCp(t, $to.parentOffset),
      }
    }
    this.ui.cursor = {
      key,
      id: this.sync.idOf(key),
      index: $head.index(0),
      kind: para.attrs.kind as ParagraphKind,
      cp,
      text,
      sentence: h ? h.sentence : null,
      half: h ? h.index : null,
      selection,
      crossParagraph,
    }
  }

  // ---------- 悬停 ----------

  private hoverFrame = 0
  private trackHover(ev: MouseEvent): void {
    if (this.hoverFrame) return
    this.hoverFrame = requestAnimationFrame(() => {
      this.hoverFrame = 0
      const view = this.view
      if (!view) return
      const target = ev.target as HTMLElement | null
      if (!target || !target.closest('.sk-text')) {
        this.setHover(null)
        return
      }
      const r = view.posAtCoords({ left: ev.clientX, top: ev.clientY })
      if (!r) return this.setHover(null)
      const $p = view.state.doc.resolve(r.pos)
      if ($p.parent.type !== schema.nodes.paragraph) return this.setHover(null)
      const text = $p.parent.textContent
      this.setHover({ key: $p.parent.attrs.key as string, cp: u16ToCp(text, $p.parentOffset) })
    })
  }

  private setHover(h: { key: string; cp: number } | null): void {
    const old = this.ui.hover
    if (old?.key === h?.key && old?.cp === h?.cp) return
    this.ui.hover = h
    this.refresh()
  }

  // ---------- 段落操作 ----------

  setKind(key: string, kind: ParagraphKind): void {
    const f = this.find(key)
    const view = this.view
    if (!f || !view) return
    view.dispatch(view.state.tr.setNodeMarkup(f.pos, undefined, { ...f.node.attrs, kind }))
  }

  insertAfter(key: string, kind: ParagraphKind): void {
    const f = this.find(key)
    const view = this.view
    if (!f || !view) return
    view.dispatch(insertParagraphAfter(view.state, f.pos, kind))
    view.focus()
  }

  deleteParagraph(key: string): void {
    const f = this.find(key)
    const view = this.view
    if (!f || !view) return
    let tr = view.state.tr
    if (view.state.doc.childCount === 1) {
      tr = tr.replaceWith(f.pos, f.pos + f.node.nodeSize, makeParagraph('normal'))
    } else {
      tr = tr.delete(f.pos, f.pos + f.node.nodeSize)
    }
    view.dispatch(tr.scrollIntoView())
  }

  moveCurrent(delta: number): boolean {
    const view = this.view
    if (!view) return false
    const from = paragraphIndexAt(view.state, view.state.selection.from)
    const tr = moveParagraph(view.state, from, from + delta)
    if (tr) view.dispatch(tr)
    return true
  }

  moveKey(key: string, delta: number): void {
    const f = this.find(key)
    const view = this.view
    if (!f || !view) return
    const tr = moveParagraph(view.state, f.index, f.index + delta)
    if (tr) view.dispatch(tr)
  }

  // ---------- 拖拽排序 ----------

  private startDrag(key: string, ev: DragEvent): void {
    this.dragKey = key
    if (ev.dataTransfer) {
      ev.dataTransfer.effectAllowed = 'move'
      ev.dataTransfer.setData('application/x-scribekit-paragraph', key)
      const f = this.find(key)
      const dom = f && this.view ? (this.view.nodeDOM(f.pos) as HTMLElement | null) : null
      if (dom) ev.dataTransfer.setDragImage(dom, 12, 12)
    }
  }

  private endDrag(): void {
    this.dragKey = null
    if (this.ui.drop) {
      this.ui.drop = null
      this.refresh()
    }
  }

  private dropTarget(ev: DragEvent): { key: string; side: 'before' | 'after' } | null {
    const paraEl = (ev.target as HTMLElement | null)?.closest('.sk-para') as HTMLElement | null
    if (!paraEl?.dataset.key) return null
    const rect = paraEl.getBoundingClientRect()
    return { key: paraEl.dataset.key, side: ev.clientY < rect.top + rect.height / 2 ? 'before' : 'after' }
  }

  private dragOver(ev: DragEvent): boolean {
    if (!this.dragKey) return false
    ev.preventDefault()
    if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'move'
    const t = this.dropTarget(ev)
    if (t?.key !== this.ui.drop?.key || t?.side !== this.ui.drop?.side) {
      this.ui.drop = t
      this.refresh()
    }
    return true
  }

  private dropOn(ev: DragEvent): boolean {
    if (!this.dragKey) return false
    ev.preventDefault()
    const t = this.dropTarget(ev)
    const key = this.dragKey
    this.endDrag()
    const view = this.view
    if (!t || !view || t.key === key) return true
    const from = this.find(key)
    const to = this.find(t.key)
    if (!from || !to) return true
    let target = to.index + (t.side === 'after' ? 1 : 0)
    if (from.index < target) target--
    const tr = moveParagraph(view.state, from.index, target)
    if (tr) view.dispatch(tr)
    return true
  }

  // ---------- 定位与远程更新 ----------

  /** 选中段落内的码点区间并滚动到可见。 */
  reveal(key: string, start: number, end = start): void {
    const f = this.find(key)
    const view = this.view
    if (!f || !view) return
    const text = f.node.textContent
    const a = f.pos + 1 + cpToU16(text, start)
    const b = f.pos + 1 + cpToU16(text, end)
    view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc, a, b)).scrollIntoView())
    view.focus()
  }

  revealId(id: number, start: number, end = start): void {
    const key = this.sync.keyOf(id)
    if (key) this.reveal(key, start, end)
  }

  /**
   * 合入建议后后端返回受影响的段落：已有的替换正文与类型，新段落插到 afterParagraphId 之后。
   * 这次修改不再回写后端（REMOTE），但会进入撤销历史，撤销后按普通编辑同步。
   */
  applyServerParagraphs(paragraphs: Paragraph[], afterParagraphId: number | null | undefined): void {
    const view = this.view
    if (!view) return
    let tr = view.state.tr.setMeta(REMOTE, true)
    const fresh: Paragraph[] = []
    for (const p of paragraphs) {
      const key = this.sync.keyOf(p.id)
      if (!key) {
        fresh.push(p)
        continue
      }
      let pos = -1
      tr.doc.forEach((n, off) => {
        if (n.attrs.key === key) pos = off
      })
      if (pos < 0) continue
      const node = tr.doc.nodeAt(pos)!
      tr = tr.replaceWith(pos, pos + node.nodeSize, makeParagraph(p.kind, p.text, key))
    }
    let insertAt = 0
    if (afterParagraphId !== null && afterParagraphId !== undefined) {
      const k = this.sync.keyOf(afterParagraphId)
      tr.doc.forEach((n, off) => {
        if (n.attrs.key === k) insertAt = off + n.nodeSize
      })
    }
    for (const p of fresh) {
      const node = makeParagraph(p.kind, p.text, keyForId(p.id))
      tr = tr.insert(insertAt, node)
      insertAt += node.nodeSize
    }
    this.sync.absorb(paragraphs, (p) => keyForId(p.id))
    view.dispatch(tr.scrollIntoView())
  }

  /** 放弃本地改动，按服务器版本重建。 */
  reload(detail: ArticleDetail): void {
    const { paragraphs, ...meta } = detail
    Object.assign(this.meta, meta)
    this.sync.reset(rowsFrom(paragraphs))
    const state = this.createState(paragraphs)
    this.state = state
    this.view?.updateState(state)
    this.updateCounts()
    this.updateCursor()
    this.ui.revision++
  }

  focus(): void {
    this.view?.focus()
  }

  /** 光标处的屏幕坐标，命令面板据此定位。 */
  caretCoords(): { left: number; top: number; bottom: number } | null {
    const view = this.view
    if (!view) return null
    try {
      return view.coordsAtPos(view.state.selection.head)
    } catch {
      return null
    }
  }
}

function changedKeys(a: PMNode, b: PMNode): string[] {
  const old = new Map<string, string>()
  a.forEach((n) => old.set(n.attrs.key as string, n.textContent))
  const out: string[] = []
  b.forEach((n) => {
    const k = n.attrs.key as string
    if (old.get(k) !== n.textContent) out.push(k)
  })
  return out
}
