import { describe, expect, test } from 'bun:test'
import type { SuggestionHalf } from '../api/types'
import { canApply, finalText, undecidedCount } from './review'

const h = (index: number, text: string, state: SuggestionHalf['state'], edited?: string): SuggestionHalf => ({
  index,
  start: 0,
  end: 0,
  text,
  state,
  edited,
})

describe('建议评审合成', () => {
  test('拼接规则', () => {
    const halves = [h(0, '风很大，', 'accepted'), h(1, '树在摇，', 'rejected'), h(2, '他走着。', 'edited', '他低头走着。')]
    expect(finalText(halves)).toBe('风很大，他低头走着。')
    expect(canApply(halves)).toBe(true)
  })

  test('有未决定的半句不能合入', () => {
    const halves = [h(0, '风很大，', 'accepted'), h(1, '树在摇。', 'pending')]
    expect(undecidedCount(halves)).toBe(1)
    expect(canApply(halves)).toBe(false)
  })

  test('全部拒绝时没有可合入的文本', () => {
    expect(canApply([h(0, '风很大。', 'rejected')])).toBe(false)
    expect(canApply([])).toBe(false)
  })
})
