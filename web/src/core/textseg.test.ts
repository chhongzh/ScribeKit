import { describe, expect, test } from 'bun:test'
import { halfAt, segment } from './textseg'

function bars(text: string): string {
  const cps = Array.from(text)
  return segment(text)
    .halves.map((h) => cps.slice(h.start, h.end).join(''))
    .join('|')
}

describe('SPEC §3 测试向量', () => {
  test('引号与无标点结尾', () => {
    const text = '我走了，他说：“你去吧。”然后'
    expect(bars(text)).toBe('我走了，|他说：|“你去吧。”|然后')
    const seg = segment(text)
    expect(seg.sentences.map((s) => s.halves)).toEqual([[0, 1, 2], [3]])
  })

  test('顿号不切分', () => {
    expect(bars('春天、夏天，都好。')).toBe('春天、夏天，|都好。')
  })

  test('连续省略号是一个句末', () => {
    const seg = segment('啊……他愣住了。')
    expect(bars('啊……他愣住了。')).toBe('啊……|他愣住了。')
    expect(seg.sentences.length).toBe(2)
  })

  test('开头空白跳过', () => {
    const text = '  空格开头，结尾无标点'
    expect(bars(text)).toBe('空格开头，|结尾无标点')
    expect(segment(text).halves[0]!.start).toBe(2)
  })

  test('段内换行结束半句但不结束句子', () => {
    const text = '山那边\n是海，\n是海'
    expect(bars(text)).toBe('山那边|是海，|是海')
    expect(segment(text).sentences.map((s) => s.halves)).toEqual([[0, 1, 2]])
    expect(segment(text).halves[1]).toMatchObject({ start: 4, end: 7 })
  })
})

describe('补充行为', () => {
  test('纯标点片段并入前一个半句', () => {
    expect(bars('真的吗？！好。')).toBe('真的吗？！|好。')
    expect(segment('真的吗？！好。').sentences.length).toBe(2)
  })

  test('空文本', () => {
    expect(segment('')).toEqual({ sentences: [], halves: [] })
    expect(segment('   ')).toEqual({ sentences: [], halves: [] })
  })

  test('码点偏移：扩展区汉字算一个码点', () => {
    const seg = segment('𠀀𠀁，好')
    expect(seg.halves[0]).toEqual({ index: 0, start: 0, end: 3, sentence: 0 })
    expect(seg.halves[1]).toEqual({ index: 1, start: 3, end: 4, sentence: 0 })
  })

  test('index 连续，sentence 指向正确', () => {
    const seg = segment('一，二。三，四。')
    expect(seg.halves.map((h) => h.sentence)).toEqual([0, 0, 1, 1])
    expect(seg.sentences.map((s) => [s.start, s.end])).toEqual([
      [0, 4],
      [4, 8],
    ])
  })

  test('halfAt', () => {
    const seg = segment('我走了，他说')
    expect(halfAt(seg, 0)!.index).toBe(0)
    expect(halfAt(seg, 4)!.index).toBe(1)
    expect(halfAt(seg, 6)!.index).toBe(1)
    expect(halfAt(segment(''), 0)).toBeNull()
  })
})
