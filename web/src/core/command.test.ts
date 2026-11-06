import { describe, expect, test } from 'bun:test'
import { findCommands, pickCommand } from './command'

describe('[ ... ] 命令', () => {
  test('识别方括号与中文方头括号', () => {
    const cmds = findCommands('他推开门。[补一段环境描写]然后【 写他的神态 】')
    expect(cmds.map((c) => c.instruction)).toEqual(['补一段环境描写', '写他的神态'])
    expect(cmds[0]).toMatchObject({ start: 5, end: 14 })
  })

  test('空命令忽略', () => {
    expect(findCommands('[] 【  】')).toEqual([])
  })

  test('码点偏移', () => {
    expect(findCommands('𠀀[写]')[0]).toMatchObject({ start: 1, end: 4 })
  })

  test('选择规则', () => {
    const cmds = findCommands('[甲]中间[乙]尾巴[丙]')
    expect(pickCommand(cmds, 6)!.instruction).toBe('乙')
    expect(pickCommand(cmds, 8)!.instruction).toBe('乙')
    expect(pickCommand(cmds, 0)!.instruction).toBe('甲')
    expect(pickCommand([], 0)).toBeNull()
  })
})
