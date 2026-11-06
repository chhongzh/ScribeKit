// SPEC §3 半句切分。所有偏移是码点下标，区间左闭右开。
// 必须与 internal/textseg 的 Go 实现逐字一致，改动时两边同时改并跑同一组测试向量。

export interface SegSentence {
  index: number
  start: number
  end: number
  halves: number[]
}

export interface SegHalf {
  index: number
  start: number
  end: number
  sentence: number
}

export interface Segmentation {
  sentences: SegSentence[]
  halves: SegHalf[]
}

const SENTENCE_END = new Set(['。', '！', '？', '!', '?'])
const ELLIPSIS = '…'
const HALF_SEP = new Set(['，', '；', '：', ',', ';', ':'])
const CLOSERS = new Set(['”', '’', '」', '』', '）', ')', '》', '】', '"', "'"])
const LEADING_SPACE = new Set([' ', '\u3000', '\t'])
const LINE_BREAK = '\n'
const BLANK_OR_PUNCT = /^[\s\p{P}\p{S}]*$/u

interface Piece {
  start: number
  end: number
  endsSentence: boolean
}

export function segment(text: string): Segmentation {
  const cps = Array.from(text)
  const pieces: Piece[] = []
  let i = 0
  const n = cps.length

  while (i < n) {
    while (i < n && (LEADING_SPACE.has(cps[i]!) || cps[i] === LINE_BREAK)) i++
    if (i >= n) break
    const start = i
    let endsSentence = false
    while (i < n) {
      const c = cps[i]!
      // 规则 8：段内换行结束当前半句但不结束句子，换行符本身不属于任何片段。
      if (c === LINE_BREAK) break
      if (c === ELLIPSIS) {
        while (i < n && cps[i] === ELLIPSIS) i++
        endsSentence = true
        break
      }
      if (SENTENCE_END.has(c)) {
        i++
        endsSentence = true
        break
      }
      if (HALF_SEP.has(c)) {
        i++
        break
      }
      i++
    }
    if (i > start && (endsSentence || HALF_SEP.has(cps[i - 1]!))) {
      while (i < n && CLOSERS.has(cps[i]!)) i++
    }
    const body = cps.slice(start, i).join('')
    if (BLANK_OR_PUNCT.test(body)) {
      // 规则 7：纯标点并入前一个片段的结尾，没有前一个片段就丢弃；纯空白直接丢弃。
      const prev = pieces[pieces.length - 1]
      if (prev && body.trim() !== '' && /[\p{P}\p{S}]/u.test(body)) {
        prev.end = i
        if (endsSentence) prev.endsSentence = true
      }
      continue
    }
    pieces.push({ start, end: i, endsSentence })
  }

  const halves: SegHalf[] = []
  const sentences: SegSentence[] = []
  let current: SegSentence | null = null
  pieces.forEach((p, idx) => {
    if (!current) {
      current = { index: sentences.length, start: p.start, end: p.end, halves: [] }
      sentences.push(current)
    }
    current.halves.push(idx)
    current.end = p.end
    halves.push({ index: idx, start: p.start, end: p.end, sentence: current.index })
    if (p.endsSentence) current = null
  })
  return { sentences, halves }
}

/** 光标（码点偏移）所在的半句。落在两个半句之间的空白时归入前一个半句，在开头空白里归入第一个。 */
export function halfAt(seg: Segmentation, offset: number): SegHalf | null {
  if (seg.halves.length === 0) return null
  let found: SegHalf | null = null
  for (const h of seg.halves) {
    if (offset >= h.start) found = h
    else break
  }
  return found ?? seg.halves[0]!
}
