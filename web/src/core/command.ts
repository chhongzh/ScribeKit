import { u16ToCp } from './offsets'

// 段落里的 [ ... ] 是写给 Scaffolder 的指令，不是正文。
// 中文输入法下按 [ 通常得到【，两种写法都识别；instruction 取括号内去掉首尾空白的文字。

export interface CommandToken {
  start: number // 码点，含左括号
  end: number // 码点，含右括号
  instruction: string
}

const PATTERN = /\[([^[\]\n]*)\]|【([^【】\n]*)】/g

export function findCommands(text: string): CommandToken[] {
  const out: CommandToken[] = []
  for (const m of text.matchAll(PATTERN)) {
    const inner = (m[1] ?? m[2] ?? '').trim()
    if (inner === '') continue
    const u = m.index ?? 0
    out.push({ start: u16ToCp(text, u), end: u16ToCp(text, u + m[0].length), instruction: inner })
  }
  return out
}

/** 去掉全部 [ ] 指令，剩下的才是正文（字数统计用）。 */
export function stripCommands(text: string): string {
  return text.replace(PATTERN, (m, a?: string, b?: string) => ((a ?? b ?? '').trim() === '' ? m : ''))
}

const HALF_END =/[。！？!?…，；：,;:”’」』）)》】"'\n]$/u

/** 光标前是否有一个还没写完的半句（续写的前提）。cursor 为码点偏移。 */
export function hasUnfinishedHalf(text: string, cursor: number): boolean {
  const before = Array.from(text).slice(0, cursor).join('')
  const stripped = before.replace(/\[[^[\]\n]*\]|【[^【】\n]*】/g, '').replace(/[ \t　]+$/u, '')
  if (stripped === '') return false
  return !HALF_END.test(stripped)
}

/** Shift+Enter 执行哪条命令：光标在命令内或紧贴其后优先，其次是光标前最近的一条，再其次是第一条。 */
export function pickCommand(tokens: CommandToken[], cursor: number): CommandToken | null {
  if (tokens.length === 0) return null
  const inside = tokens.find((t) => cursor >= t.start && cursor <= t.end)
  if (inside) return inside
  const before = tokens.filter((t) => t.end <= cursor)
  if (before.length > 0) return before[before.length - 1]!
  return tokens[0]!
}
