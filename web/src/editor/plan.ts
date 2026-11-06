import type { ParagraphKind } from '../api/types'
import { cpLength } from '../core/offsets'

// 持久化规划：比较“服务器上已知的段落序列”与“编辑器里的段落序列”，每次给出一步要调用的接口。
// 只看两边的差，不看用户按了什么键，所以撤销、粘贴、跨段删除都走同一条路。
// 段落用本地 key 标识（新段落在拿到服务器 id 之前就有 key），id 只存在服务器一侧的行里。

export interface ServerRow {
  key: string
  id: number
  kind: ParagraphKind
  text: string
  position: number
}

export interface LocalRow {
  key: string
  kind: ParagraphKind
  text: string
}

export type Op =
  | { t: 'merge'; key: string; removed: string }
  | { t: 'delete'; key: string }
  | { t: 'split'; key: string; newKey: string; offset: number; kind: ParagraphKind }
  | { t: 'create'; key: string; afterKey: string | null; kind: ParagraphKind; text: string }
  | { t: 'move'; key: string; afterKey: string | null }
  | { t: 'kind'; key: string; kind: ParagraphKind }
  | { t: 'text'; key: string; text: string }

export interface PlanOptions {
  /** 是否同步正文改动。打字时防抖，结构变化立即同步。true = 全部，Set = 只同步这些段落。 */
  text: boolean | Set<string>
}

/** 空文章打开时编辑器里放的那个空段落，用户写字之前不入库。 */
function isPlaceholder(server: ServerRow[], local: LocalRow[]): boolean {
  return server.length === 0 && local.length === 1 && local[0]!.text === ''
}

export function nextOp(server: ServerRow[], local: LocalRow[], opts: PlanOptions): Op | null {
  if (isPlaceholder(server, local)) return null
  const localByKey = new Map(local.map((r) => [r.key, r]))
  const serverByKey = new Map(server.map((r) => [r.key, r]))

  // 1. 编辑器里已经没有的段落：能看出是并入了上一段就合并，否则删除。
  for (let i = 0; i < server.length; i++) {
    const r = server[i]!
    if (localByKey.has(r.key)) continue
    const prev = i > 0 ? server[i - 1]! : null
    const prevLocal = prev ? localByKey.get(prev.key) : undefined
    if (prev && prevLocal && (r.text === '' || prevLocal.text.endsWith(r.text))) {
      // 合并前先让上一段的服务器文本等于“本地文本去掉被并入的那部分”，merge-next 的结果才和本地一致。
      const head = r.text === '' ? prevLocal.text : prevLocal.text.slice(0, prevLocal.text.length - r.text.length)
      if (prev.text !== head) return { t: 'text', key: prev.key, text: head }
      return { t: 'merge', key: prev.key, removed: r.key }
    }
    return { t: 'delete', key: r.key }
  }

  // 2. 服务器上还没有的段落：紧跟在被改短的段落后面的非空段视为拆分，否则新建。
  for (let i = 0; i < local.length; i++) {
    const a = local[i]!
    if (serverByKey.has(a.key)) continue
    const prevLocal = i > 0 ? local[i - 1]! : null
    const prevServer = prevLocal ? serverByKey.get(prevLocal.key) : undefined
    if (prevLocal && prevServer && a.text !== '') {
      const whole = prevLocal.text + a.text
      if (prevServer.text === whole) {
        return { t: 'split', key: prevLocal.key, newKey: a.key, offset: cpLength(prevLocal.text), kind: a.kind }
      }
      if (prevServer.text !== prevLocal.text) return { t: 'text', key: prevLocal.key, text: whole }
    }
    let afterKey: string | null = null
    for (let j = i - 1; j >= 0; j--) {
      if (serverByKey.has(local[j]!.key)) {
        afterKey = local[j]!.key
        break
      }
    }
    return { t: 'create', key: a.key, afterKey, kind: a.kind, text: a.text }
  }

  // 3. 顺序：保留最长的已有序子序列不动，把其余段落逐个移到本地前一段之后。
  const order = local.map((r) => r.key)
  const serverIndex = new Map(server.map((r, i) => [r.key, i]))
  const seq = order.map((k) => serverIndex.get(k)!)
  if (seq.some((v, i) => i > 0 && v < seq[i - 1]!)) {
    const keep = longestIncreasing(seq)
    for (let i = 0; i < order.length; i++) {
      if (!keep.has(i)) return { t: 'move', key: order[i]!, afterKey: i > 0 ? order[i - 1]! : null }
    }
  }

  // 4. 段落类型立即同步。
  for (const l of local) {
    const s = serverByKey.get(l.key)!
    if (s.kind !== l.kind) return { t: 'kind', key: l.key, kind: l.kind }
  }

  // 5. 正文。
  for (const l of local) {
    const s = serverByKey.get(l.key)!
    if (s.text === l.text) continue
    if (opts.text === true || (opts.text instanceof Set && opts.text.has(l.key))) return { t: 'text', key: l.key, text: l.text }
  }
  return null
}

/** 最长严格递增子序列的下标集合。 */
export function longestIncreasing(seq: number[]): Set<number> {
  const tails: number[] = []
  const prev: number[] = new Array(seq.length).fill(-1)
  for (let i = 0; i < seq.length; i++) {
    let lo = 0
    let hi = tails.length
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (seq[tails[mid]!]! < seq[i]!) lo = mid + 1
      else hi = mid
    }
    if (lo > 0) prev[i] = tails[lo - 1]!
    tails[lo] = i
  }
  const out = new Set<number>()
  let k = tails.length > 0 ? tails[tails.length - 1]! : -1
  while (k >= 0) {
    out.add(k)
    k = prev[k]!
  }
  return out
}

/** 结构是否一致（不看正文）。用于判断还有没有必须立即同步的改动。 */
export function structureDiffers(server: ServerRow[], local: LocalRow[]): boolean {
  return nextOp(server, local, { text: false }) !== null
}
