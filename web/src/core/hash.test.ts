import { describe, expect, test } from 'bun:test'
import { createHash } from 'node:crypto'
import { sha256Hex, textHash } from './hash'

describe('sha256', () => {
  test('与平台实现一致', () => {
    for (const s of ['', 'abc', '我走了，他说：“你去吧。”然后', '𠀀😀', 'x'.repeat(1000)]) {
      expect(sha256Hex(s)).toBe(createHash('sha256').update(s, 'utf8').digest('hex'))
    }
  })
  test('textHash 取前 16 位', () => {
    expect(textHash('abc')).toBe(createHash('sha256').update('abc').digest('hex').slice(0, 16))
  })
})
