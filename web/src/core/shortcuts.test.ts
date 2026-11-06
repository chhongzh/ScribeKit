import { expect, test } from 'bun:test'
import { globalShortcut } from './shortcuts'

function key(code: string, modifiers = {}) {
  return { code, ctrlKey: true, metaKey: false, shiftKey: false, altKey: false, isComposing: false, keyCode: 0, ...modifiers }
}

test('区分浏览器冲突快捷键，检查器不会误触侧边栏', () => {
  expect(globalShortcut(key('KeyB'))).toBe('sidebar')
  expect(globalShortcut(key('KeyB', { altKey: true }))).toBe('inspector')
  expect(globalShortcut(key('KeyJ', { shiftKey: true }))).toBeNull()
  expect(globalShortcut(key('KeyK', { shiftKey: true }))).toBe('palette')
  expect(globalShortcut(key('KeyE', { shiftKey: true }))).toBe('explorer')
  expect(globalShortcut(key('KeyS', { ctrlKey: false, metaKey: true }))).toBe('save')
})

test('输入法组合阶段和额外修饰键不触发全局操作', () => {
  expect(globalShortcut(key('KeyK', { isComposing: true }))).toBeNull()
  expect(globalShortcut(key('KeyK', { keyCode: 229 }))).toBeNull()
  expect(globalShortcut(key('KeyK', { altKey: true }))).toBeNull()
  expect(globalShortcut(key('KeyB', { ctrlKey: false }))).toBeNull()
})
