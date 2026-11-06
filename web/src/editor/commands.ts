import type { Command, EditorState, Transaction } from 'prosemirror-state'
import { TextSelection } from 'prosemirror-state'
import type { ParagraphKind } from '../api/types'
import { makeParagraph, newKey, schema } from './schema'

// Enter = 确认放下：在光标处断开成两段，新段默认普通段。
// 段尾回车新建空段落，段首回车在上方插入空段落、原段落保持原样（类型、id 都不动）。
export const enterCommand: Command = (state, dispatch) => {
  let tr = state.tr
  if (!state.selection.empty) tr = tr.deleteSelection()
  const $pos = tr.selection.$from
  if ($pos.parent.type !== schema.nodes.paragraph) return false
  const para = $pos.parent
  const offset = $pos.parentOffset
  const start = $pos.before()
  const end = $pos.after()
  if (offset === 0 && para.content.size > 0) {
    tr = tr.insert(start, makeParagraph('normal'))
    tr = tr.setSelection(TextSelection.create(tr.doc, start + 1 + 2))
  } else if (offset === para.content.size) {
    tr = tr.insert(end, makeParagraph('normal'))
    tr = tr.setSelection(TextSelection.create(tr.doc, end + 1))
  } else {
    tr = tr.split($pos.pos, 1, [{ type: schema.nodes.paragraph!, attrs: { key: newKey(), kind: 'normal' } }])
  }
  dispatch?.(tr.scrollIntoView())
  return true
}

/** 在指定段落之后插入一个指定类型的空段落，并把光标放进去。 */
export function insertParagraphAfter(state: EditorState, pos: number, kind: ParagraphKind): Transaction {
  const node = state.doc.nodeAt(pos)
  const end = pos + (node?.nodeSize ?? 0)
  let tr = state.tr.insert(end, makeParagraph(kind))
  tr = tr.setSelection(TextSelection.create(tr.doc, end + 1))
  return tr.scrollIntoView()
}

/** 把下标 from 的段落移动到下标 to（以移动后的顺序计）。 */
export function moveParagraph(state: EditorState, from: number, to: number): Transaction | null {
  const count = state.doc.childCount
  if (from === to || from < 0 || to < 0 || from >= count || to >= count) return null
  const node = state.doc.child(from)
  let start = 0
  for (let i = 0; i < from; i++) start += state.doc.child(i).nodeSize
  const sel = state.selection
  const inside = sel.from >= start && sel.to <= start + node.nodeSize
  const relHead = sel.head - start
  let tr = state.tr.delete(start, start + node.nodeSize)
  let target = 0
  for (let i = 0; i < to; i++) target += tr.doc.child(i).nodeSize
  tr = tr.insert(target, node)
  if (inside) tr = tr.setSelection(TextSelection.create(tr.doc, Math.min(target + relHead, target + node.nodeSize - 1)))
  return tr.scrollIntoView()
}

/** 段落序号（文档里的第几个子节点）。 */
export function paragraphIndexAt(state: EditorState, pos: number): number {
  return state.doc.resolve(Math.min(pos, state.doc.content.size)).index(0)
}
