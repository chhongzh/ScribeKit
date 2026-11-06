import { Schema, type Node as PMNode } from 'prosemirror-model'
import type { ParagraphKind } from '../api/types'

// 文档 = 段落序列，段落内只有纯文本，没有任何 mark。
// 高亮、命令标记、建议都是 Decoration，不进入文本，所以段落文本与服务器上的 text 逐字相同。
// attrs.key 是本地稳定标识（已入库段落为 "p<id>"），服务器 id 由 SyncEngine 按 key 查；
// 不把 id 直接放进 attrs，是因为撤销会让已删除的节点连同旧 attrs 一起复活。

let seq = 0
export function newKey(): string {
  seq++
  return `n${Date.now().toString(36)}${seq.toString(36)}`
}

export function keyForId(id: number): string {
  return `p${id}`
}

const KINDS = new Set<string>(['opening', 'closing', 'callback', 'lyric', 'transition', 'normal'])

export const schema = new Schema({
  nodes: {
    doc: { content: 'paragraph+' },
    paragraph: {
      content: 'inline*',
      marks: '',
      attrs: { key: { default: '' }, kind: { default: 'normal' } },
      draggable: false,
      parseDOM: [
        {
          tag: 'p',
          // 粘贴进来的段落一律拿新 key，避免与已有段落重复。
          getAttrs: (dom) => {
            const k = (dom as HTMLElement).getAttribute('data-kind') ?? 'normal'
            return { key: newKey(), kind: KINDS.has(k) ? k : 'normal' }
          },
        },
        { tag: 'div.sk-para', getAttrs: () => ({ key: newKey(), kind: 'normal' }) },
      ],
      toDOM: (node) => ['p', { 'data-kind': node.attrs.kind as string }, 0],
    },
    text: { group: 'inline' },
    // 段内换行（诗歌分行）。leafText 让 textContent 里它就是 '\n'，且只占 1 个位置，
    // 所以“段内 UTF-16 下标 = ProseMirror 段内偏移”这一点不受影响。
    hard_break: {
      inline: true,
      group: 'inline',
      selectable: false,
      leafText: () => '\n',
      parseDOM: [{ tag: 'br' }],
      toDOM: () => ['br'],
    },
  },
  marks: {},
})

/** 段落文本（可含 '\n'）→ 节点内容。 */
export function inlineContent(text: string): PMNode[] {
  const out: PMNode[] = []
  text.split('\n').forEach((line, i) => {
    if (i > 0) out.push(schema.nodes.hard_break!.create())
    if (line) out.push(schema.text(line))
  })
  return out
}

export function paragraphKind(node: PMNode): ParagraphKind {
  return node.attrs.kind as ParagraphKind
}

export function paragraphKey(node: PMNode): string {
  return node.attrs.key as string
}

export function makeParagraph(kind: ParagraphKind, text = '', key = newKey()): PMNode {
  return schema.nodes.paragraph!.create({ key, kind }, inlineContent(text))
}
