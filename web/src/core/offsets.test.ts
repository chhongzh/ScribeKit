import { describe, expect, test } from 'bun:test'
import { countChars, cpLength, cpToU16, sliceCp, u16ToCp } from './offsets'

describe('码点与 UTF-16 换算', () => {
  test('纯 BMP 文本两种下标相同', () => {
    const t = '我走了，他说'
    for (let i = 0; i <= t.length; i++) {
      expect(cpToU16(t, i)).toBe(i)
      expect(u16ToCp(t, i)).toBe(i)
    }
  })

  test('扩展区汉字与 emoji 各占两个 UTF-16 单元', () => {
    const t = 'a𠀀b😀c'
    expect(t.length).toBe(7)
    expect(cpLength(t)).toBe(5)
    expect([0, 1, 2, 3, 4, 5].map((c) => cpToU16(t, c))).toEqual([0, 1, 3, 4, 6, 7])
    expect([0, 1, 3, 4, 6, 7].map((u) => u16ToCp(t, u))).toEqual([0, 1, 2, 3, 4, 5])
  })

  test('代理对中间向前取整', () => {
    expect(u16ToCp('a𠀀b', 2)).toBe(1)
  })

  test('越界夹取', () => {
    expect(cpToU16('ab', 99)).toBe(2)
    expect(cpToU16('ab', -3)).toBe(0)
    expect(u16ToCp('ab', 99)).toBe(2)
  })

  test('往返一致', () => {
    const t = '𠀀，😀😀。好'
    for (let c = 0; c <= cpLength(t); c++) expect(u16ToCp(t, cpToU16(t, c))).toBe(c)
  })

  test('孤立代理按一个码点计，与 Array.from 一致', () => {
    const t = 'a\ud800b'
    expect(cpLength(t)).toBe(3)
    expect(Array.from(t).length).toBe(3)
  })

  test('sliceCp 与 countChars', () => {
    expect(sliceCp('a𠀀b😀c', 1, 4)).toBe('𠀀b😀')
    expect(countChars('  我 走　了 ')).toBe(3)
  })
})
