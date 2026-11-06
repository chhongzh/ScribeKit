import type { ParagraphKind, ScaffoldMode } from '../api/types'
import { findCommands, hasUnfinishedHalf, pickCommand, type CommandToken } from '../core/command'
import { isUserOwnedKind, kindLabel } from '../core/labels'
import { cpLength, sliceCp } from '../core/offsets'
import type { ScaffoldDraft, StreamView } from '../stores/suggestions'
import type { ArticleSession, CursorInfo } from './session'

// 把“光标即命令面板”的各种入口统一成一个请求草稿：发什么 mode、带哪些上下文、流式文字显示在哪。

export interface ActionInput {
  mode: ScaffoldMode
  instruction: string
  newParagraphKind?: ParagraphKind | null
  command?: CommandToken | null
  force?: boolean
  title: string
}

export interface Prepared {
  draft: ScaffoldDraft
  view: StreamView
}

export function blockedReason(session: ArticleSession): string | null {
  if (!session.meta.title.trim() || !session.meta.synopsis.trim()) return '先写下标题和梗概，Scaffolder 才有东西可接'
  return null
}

export function prepare(session: ArticleSession, c: CursorInfo, a: ActionInput): Prepared {
  const sel = c.selection && (a.mode === 'polish' || a.mode === 'detail' || a.mode === 'generate' || a.mode === 'advise') ? c.selection : null
  const cursorOffset = a.command ? a.command.end : c.cp
  const draft: ScaffoldDraft = {
    cursor: { key: c.key, offset: cursorOffset },
    selection: sel,
    commandRange: a.command ? { key: c.key, start: a.command.start, end: a.command.end } : null,
    instruction: a.instruction,
    mode: a.mode,
    newParagraphKind: a.newParagraphKind ?? null,
    force: a.force ?? false,
  }
  const block = !!a.newParagraphKind || a.mode === 'advise'
  const view: StreamView = {
    title: a.title,
    anchorKey: c.key,
    anchorCp: sel ? sel.end : cursorOffset,
    block,
    paragraphKind: a.newParagraphKind ?? c.kind,
  }
  return { draft, view }
}

export type ShiftEnterPlan =
  | { kind: 'command'; action: ActionInput }
  | { kind: 'complete'; action: ActionInput }
  | { kind: 'palette'; reason: string; selection: boolean }

/** Shift+Enter：[ ] 指令优先，其次续写未完成的半句；有选区或两者都没有时打开命令面板。 */
export function planShiftEnter(c: CursorInfo): ShiftEnterPlan {
  if (c.crossParagraph) return { kind: 'palette', reason: '选区跨了段落。Scaffolder 一次只处理一段里的选区，先缩小选区。', selection: false }
  if (c.selection && c.selection.end > c.selection.start) {
    return { kind: 'palette', reason: '对选中的文字做什么？', selection: true }
  }
  const cmd = pickCommand(findCommands(c.text), c.cp)
  if (cmd) {
    return { kind: 'command', action: { mode: 'generate', instruction: cmd.instruction, command: cmd, title: cmd.instruction } }
  }
  if (hasUnfinishedHalf(c.text, c.cp)) {
    return { kind: 'complete', action: { mode: 'complete', instruction: '', title: '续写' } }
  }
  return {
    kind: 'palette',
    reason: '光标前没有没写完的半句，这一段也没有 [ ] 指令。选一个动作，或者直接写下你要它做什么。',
    selection: false,
  }
}

/** 命令面板里每个动作会交给 Scaffolder 的上下文，原样说给用户听。 */
export function contextLines(session: ArticleSession, c: CursorInfo, a: ActionInput): string[] {
  const lines = [`标题、梗概，以及全文 ${session.ui.paragraphCount} 段正文`]
  if (session.meta.collectionId !== null) lines.push('同文集其他文章的标题与梗概、文集总纲')
  lines.push('与本文、本文集相关的素材片段，按文体匹配的写作指引')
  const preview = (s: string) => (cpLength(s) > 24 ? `${sliceCp(s, 0, 24)}…` : s)
  if (a.mode === 'complete') {
    const before = sliceCp(c.text, Math.max(0, c.cp - 24), c.cp)
    lines.push(`光标在第 ${c.index + 1} 段第 ${c.cp} 字后，续写“${before}”`)
  } else if (c.selection) {
    lines.push(`第 ${c.index + 1} 段的选区“${preview(sliceCp(c.text, c.selection.start, c.selection.end))}”`)
  } else {
    lines.push(`光标位置：第 ${c.index + 1} 段第 ${a.command ? a.command.end : c.cp} 字`)
  }
  if (a.command) lines.push(`指令 [${a.command.instruction}]，生成结果默认替换这条指令`)
  else if (a.instruction) lines.push(`你的指令：${a.instruction}`)
  if (a.newParagraphKind) lines.push(`结果作为新的${kindLabel(a.newParagraphKind)}放在第 ${c.index + 1} 段之后`)
  else lines.push(`本段类型：${kindLabel(c.kind)}`)
  return lines
}

/** 转折段 / Call Back 段默认由用户自己写，后端会降级为只给建议。 */
export function degradeNote(c: CursorInfo, a: ActionInput): string | null {
  const kind = a.newParagraphKind ?? c.kind
  if (a.mode === 'advise' || !isUserOwnedKind(kind) || a.force) return null
  return `${kindLabel(kind)}由你亲自写：这次请求会被降级为只给建议，不生成可插入的正文。确实需要它写，勾选“仍然生成”。`
}
