import type { Node as PMNode } from 'prosemirror-model'
import type { Decoration, EditorView, NodeView } from 'prosemirror-view'
import { kindLabel, kindShort, markLabel } from '../core/labels'

// 段落节点视图：左侧装订线（段落类型标签、拖动柄、论点/论据/论证标记）+ 正文。
// 装订线 contenteditable=false，事件不交给 ProseMirror。

export interface GutterCallbacks {
  onKindClick(key: string, anchor: HTMLElement): void
  onDragStart(key: string, ev: DragEvent): void
  onDragEnd(): void
}

export interface ArgMarkSpec {
  argMarks: { kind: 'claim' | 'evidence' | 'reasoning'; note?: string }[]
}

export class ParagraphView implements NodeView {
  dom: HTMLElement
  contentDOM: HTMLElement
  private kindBtn: HTMLButtonElement
  private argBox: HTMLElement
  private node: PMNode

  constructor(node: PMNode, view: EditorView, cb: GutterCallbacks) {
    this.node = node
    const dom = document.createElement('div')
    dom.className = 'sk-para'

    const gutter = document.createElement('div')
    gutter.className = 'sk-gutter'
    gutter.contentEditable = 'false'

    const drag = document.createElement('span')
    drag.className = 'sk-drag'
    drag.draggable = true
    drag.title = '拖动调整段落顺序（键盘：Alt+Shift+↑ / ↓）'
    drag.setAttribute('aria-hidden', 'true')
    drag.textContent = '⋮⋮'
    drag.addEventListener('dragstart', (ev) => cb.onDragStart(this.node.attrs.key as string, ev))
    drag.addEventListener('dragend', () => cb.onDragEnd())

    const kindBtn = document.createElement('button')
    kindBtn.type = 'button'
    kindBtn.className = 'sk-kind'
    kindBtn.tabIndex = -1
    kindBtn.addEventListener('mousedown', (ev) => ev.preventDefault())
    kindBtn.addEventListener('click', () => cb.onKindClick(this.node.attrs.key as string, kindBtn))

    const argBox = document.createElement('div')
    argBox.className = 'sk-argmarks'

    gutter.append(drag, kindBtn, argBox)

    const text = document.createElement('p')
    text.className = 'sk-text'
    text.dataset.placeholder = '从这里起笔。Enter 放下这一段，Shift+Enter 交给 Scaffolder，Ctrl+K 打开命令面板'

    dom.append(gutter, text)
    this.dom = dom
    this.contentDOM = text
    this.kindBtn = kindBtn
    this.argBox = argBox
    this.render([])
    void view
  }

  private render(decos: readonly Decoration[]) {
    const kind = this.node.attrs.kind as string
    this.dom.dataset.kind = kind
    this.dom.dataset.key = this.node.attrs.key as string
    this.dom.classList.toggle('sk-empty', this.node.content.size === 0)
    this.kindBtn.textContent = `〔${kindShort(kind)}〕`
    this.kindBtn.setAttribute('aria-label', `段落类型：${kindLabel(kind)}，点击切换`)
    this.kindBtn.title = `${kindLabel(kind)}：点击切换类型或在下方新建段落`

    const marks: ArgMarkSpec['argMarks'] = []
    for (const d of decos) {
      const spec = d.spec as Partial<ArgMarkSpec> | undefined
      if (spec?.argMarks) marks.push(...spec.argMarks)
    }
    const seen = new Set<string>()
    this.argBox.replaceChildren()
    for (const m of marks) {
      if (seen.has(m.kind)) continue
      seen.add(m.kind)
      const tag = document.createElement('span')
      tag.className = `sk-arg sk-arg-${m.kind}`
      tag.textContent = markLabel(m.kind)
      if (m.note) tag.title = m.note
      this.argBox.append(tag)
    }
  }

  update(node: PMNode, decorations: readonly Decoration[]): boolean {
    if (node.type !== this.node.type) return false
    this.node = node
    this.render(decorations)
    return true
  }

  stopEvent(event: Event): boolean {
    const t = event.target as Node | null
    return !!t && !this.contentDOM.contains(t) && this.dom.contains(t)
  }

  ignoreMutation(m: MutationRecord | { type: 'selection'; target: Node }): boolean {
    if (m.type === 'selection') return false
    return !this.contentDOM.contains(m.target)
  }
}
