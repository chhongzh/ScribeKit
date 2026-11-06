import type { SuggestionHalf } from '../api/types'

// 建议评审：最终文本 = 按顺序拼接 accepted 的原文与 edited 的改写，rejected 丢弃（SPEC §5）。

export function finalText(halves: SuggestionHalf[]): string {
  return halves
    .map((h) => {
      if (h.state === 'accepted') return h.text
      if (h.state === 'edited') return h.edited ?? ''
      return ''
    })
    .join('')
}

export function undecidedCount(halves: SuggestionHalf[]): number {
  return halves.filter((h) => h.state === 'pending').length
}

export function canApply(halves: SuggestionHalf[]): boolean {
  return halves.length > 0 && undecidedCount(halves) === 0 && finalText(halves).trim() !== ''
}
