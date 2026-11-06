import { beforeEach, expect, mock, spyOn, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import type { ArticleSession } from '../editor/session'
import type { ArticleDetail } from '../api/types'
import { api } from '../api/http'

Object.defineProperty(globalThis, 'location', { value: { protocol: 'http:', host: 'localhost' }, configurable: true })
const { tabKey, tabsToClose, useEditorStore } = await import('./editor')
type Tab = import('./editor').Tab

const tabs: Tab[] = [
  { type: 'article', id: 1 },
  { type: 'guide', id: 'narrative-basics' },
  { type: 'article', id: 2 },
  { type: 'settings', id: 'settings' },
]

beforeEach(() => setActivePinia(createPinia()))

test('close scopes use the clicked tab and preserve tab order', () => {
  const target = tabs[1]!
  expect(tabsToClose(tabs, target, 'current')).toEqual([target])
  expect(tabsToClose(tabs, target, 'right')).toEqual(tabs.slice(2))
  expect(tabsToClose(tabs, target, 'others')).toEqual([tabs[0]!, ...tabs.slice(2)])
  expect(tabsToClose(tabs, target, 'all')).toEqual(tabs)
  expect(tabsToClose(tabs, tabs.at(-1)!, 'right')).toEqual([])
  expect(tabsToClose(tabs, { type: 'article', id: 999 }, 'all')).toEqual([])
})

test('closing other tabs keeps the clicked tab and activates it when needed', async () => {
  const store = useEditorStore()
  tabs.forEach(store.activate)
  await store.closeTabs(tabs[1]!, 'others', async () => false)
  expect(store.tabs).toEqual([tabs[1]!])
  expect(store.active).toBe(tabKey(tabs[1]!))
})

test('a failed save and cancelled discard stop the batch without destroying the session', async () => {
  const store = useEditorStore()
  tabs.forEach(store.activate)
  const destroy = mock(() => {})
  const flush = mock(async () => { throw new Error('offline') })
  store.sessions.set(1, { sync: { flush }, destroy } as unknown as ArticleSession)
  const confirm = mock(async () => false)
  expect(await store.closeTabs(tabs[0]!, 'all', confirm)).toBe(false)
  expect(store.tabs).toEqual(tabs)
  expect(store.sessions.has(1)).toBe(true)
  expect(flush).toHaveBeenCalledTimes(1)
  expect(destroy).not.toHaveBeenCalled()
  expect(confirm).toHaveBeenCalledWith(tabs[0]!)
  expect(store.closingTabs).toBe(false)
})

test('confirmed discard continues closing the remaining tabs', async () => {
  const store = useEditorStore()
  tabs.forEach(store.activate)
  const destroy = mock(() => {})
  store.sessions.set(1, {
    sync: { flush: async () => { throw new Error('offline') } },
    destroy,
  } as unknown as ArticleSession)
  expect(await store.closeTabs(tabs[0]!, 'all', async () => true)).toBe(true)
  expect(store.tabs).toEqual([])
  expect(store.active).toBeNull()
  expect(store.sessions.size).toBe(0)
  expect(destroy).toHaveBeenCalledTimes(1)
})

test('a second batch cannot race a pending save', async () => {
  const store = useEditorStore()
  tabs.forEach(store.activate)
  let finishSave!: () => void
  store.sessions.set(1, {
    sync: { flush: () => new Promise<void>((resolve) => { finishSave = resolve }) },
    destroy: () => {},
  } as unknown as ArticleSession)
  const first = store.closeTabs(tabs[0]!, 'current', async () => false)
  expect(store.closingTabs).toBe(true)
  expect(await store.closeTabs(tabs[1]!, 'all', async () => false)).toBe(false)
  finishSave()
  await first
  expect(store.tabs).toEqual(tabs.slice(1))
  expect(store.closingTabs).toBe(false)
})

test('closing an article while loading does not leave a session after the response arrives', async () => {
  const store = useEditorStore()
  let finishLoad!: (detail: ArticleDetail) => void
  const request = spyOn(api, 'getArticle').mockImplementation(() => new Promise((resolve) => { finishLoad = resolve }))
  try {
    const loading = store.openArticle(1)
    await store.closeTabs(tabs[0]!, 'all', async () => false)
    finishLoad({} as ArticleDetail)
    await loading
    expect(store.tabs).toEqual([])
    expect(store.sessions.size).toBe(0)
  } finally {
    request.mockRestore()
  }
})
