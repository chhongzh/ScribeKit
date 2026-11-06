import { afterEach, beforeEach, expect, mock, spyOn, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import type { ArticleSession } from '../editor/session'
import type { Envelope } from '../api/ws'
import { textHash } from '../core/hash'

Object.defineProperty(globalThis, 'location', { value: { protocol: 'http:', host: 'localhost' }, configurable: true })
const { ws } = await import('../api/ws')
const { useAnalysisStore } = await import('./analysis')

let replies: Map<string, (msg: Envelope) => void>
let send: ReturnType<typeof spyOn<typeof ws, 'send'>>
let session: ArticleSession

beforeEach(() => {
  setActivePinia(createPinia())
  ws.tasks.items.splice(0)
  replies = new Map()
  let sequence = 0
  spyOn(ws, 'nextId').mockImplementation(() => `request-${sequence++}`)
  spyOn(ws, 'whenOpen').mockResolvedValue(true)
  spyOn(ws, 'release').mockImplementation((id) => { replies.delete(id) })
  send = spyOn(ws, 'send').mockImplementation((_type, _payload, handler, requestId) => {
    const id = requestId ?? ws.nextId()
    if (handler) replies.set(id, handler)
    return id
  })
  session = {
    articleId: 1,
    ui: { kindLocked: false },
    sync: { flush: async () => {}, idOf: () => 10 },
    textOfId: () => '测试段落。',
    localRows: () => [{ key: 'p' }],
    refresh: () => {},
  } as unknown as ArticleSession
  useAnalysisStore().attach(session)
})

afterEach(() => {
  useAnalysisStore().detach(1)
  mock.restore()
})

test('batch completion clears loading even without individual results', async () => {
  const store = useAnalysisStore()
  await store.analyzeParagraphs(session, ['p'])
  expect(store.st(1).inflight).toBe(1)
  replies.get('request-0')!({ id: 'request-0', type: 'analyze.done', payload: {} })
  expect(store.st(1).inflight).toBe(0)
})

test('same paragraph and text are not sent twice while pending', async () => {
  const store = useAnalysisStore()
  await Promise.all([store.analyzeParagraphs(session, ['p']), store.analyzeParagraphs(session, ['p'])])
  expect(send).toHaveBeenCalledTimes(1)
  expect(store.st(1).inflight).toBe(1)
})

test('timeout clears loading and permits retry', async () => {
  const store = useAnalysisStore()
  await store.analyzeParagraphs(session, ['p'])
  replies.get('request-0')!({ id: 'request-0', type: 'error', payload: { code: 'timeout' } })
  expect(store.st(1).inflight).toBe(0)
  expect(store.st(1).error).toContain('超时')
  await store.analyzeParagraphs(session, ['p'])
  expect(store.st(1).inflight).toBe(1)
  expect(store.st(1).errorCode).toBe('')
})

test('cancel releases paragraph and article requests immediately', async () => {
  const store = useAnalysisStore()
  await store.analyzeArticle(session)
  expect(store.st(1).articleLoading).toBe(true)
  store.cancel(1)
  expect(store.st(1).inflight).toBe(0)
  expect(store.st(1).articleLoading).toBe(false)
  expect(replies.size).toBe(0)
})

test('cancel during save prevents the delayed request', async () => {
  let finishSave!: () => void
  session.sync.flush = () => new Promise<void>((resolve) => { finishSave = resolve })
  const store = useAnalysisStore()
  const pending = store.analyzeArticle(session)
  expect(store.st(1).preparing).toBe(1)
  store.cancel(1)
  finishSave()
  await pending
  expect(send).not.toHaveBeenCalled()
  expect(store.st(1).articleLoading).toBe(false)
  expect(store.st(1).preparing).toBe(0)
})

test('partial results keep the batch alive until analyze.done and accept subsequent halves', async () => {
  const store = useAnalysisStore()
  await store.analyzeParagraphs(session, ['p'])
  const payload = {
    paragraphId: 10,
    textHash: textHash('测试段落。'),
    segmentation: { halves: [], sentences: [] },
    halves: [{ index: 0, desc: ['action'], rhetoric: [] }],
    relations: [], marks: [], diagnostics: [], model: 'test', partial: true,
  }
  replies.get('request-0')!({ id: 'request-0', type: 'analyze.result', payload })
  expect(store.st(1).inflight).toBe(1)
  expect(replies.has('request-0')).toBe(true)
  expect(store.byParagraph[10]?.halves).toHaveLength(1)
  replies.get('request-0')!({ id: 'request-0', type: 'analyze.result', payload: { ...payload, partial: false, halves: [...payload.halves, { index: 1, desc: ['environment'], rhetoric: [] }] } })
  expect(store.byParagraph[10]?.halves).toHaveLength(2)
  expect(store.st(1).inflight).toBe(1)
  replies.get('request-0')!({ id: 'request-0', type: 'analyze.done', payload: {} })
  expect(store.st(1).inflight).toBe(0)
  expect(replies.size).toBe(0)
})

test('a failed partial analysis can be retried with unchanged text', async () => {
  const store = useAnalysisStore()
  await store.analyzeParagraphs(session, ['p'])
  replies.get('request-0')!({ id: 'request-0', type: 'analyze.result', payload: {
    paragraphId: 10, textHash: textHash('测试段落。'), partial: true,
    segmentation: { halves: [], sentences: [] }, halves: [{ index: 0, desc: ['action'], rhetoric: [] }],
    relations: [], marks: [], diagnostics: [], model: 'test',
  } })
  replies.get('request-0')!({ id: 'request-0', type: 'error', payload: { code: 'timeout' } })
  expect(store.st(1).inflight).toBe(0)
  await store.analyzeParagraphs(session, ['p'])
  expect(send).toHaveBeenCalledTimes(2)
  expect(store.st(1).inflight).toBe(1)
})

test('type inference locks manual changes until completion and retains its result action', async () => {
  const store = useAnalysisStore()
  const open = mock(() => {})
  const pending = store.inferStructure(session, open)
  expect(session.ui.kindLocked).toBe(true)
  expect(ws.tasks.active[0]?.title).toBe('段落类型推断')
  await Bun.sleep(0)
  const result = { articleId: 1, paragraphs: [], themeWords: [], themeHits: [], structure: { name: '', fit: '', gaps: [] }, notes: [] }
  replies.get('request-0')!({ id: 'request-0', type: 'analyze.articleResult', payload: result })
  expect(await pending).toEqual(result)
  expect(session.ui.kindLocked).toBe(false)
  expect(store.st(1).articleLoading).toBe(false)
  expect(ws.tasks.items[0]?.status).toBe('completed')
  ws.tasks.items[0]?.open?.()
  expect(open).toHaveBeenCalledTimes(1)
})

test('cancelling inference during save immediately unlocks and prevents sending after save', async () => {
  let finishSave!: () => void
  session.sync.flush = () => new Promise<void>((resolve) => { finishSave = resolve })
  const store = useAnalysisStore()
  const pending = store.inferStructure(session)
  const cancelled = pending.catch((error: Error) => error)
  ws.tasks.active[0]!.cancel()
  expect(session.ui.kindLocked).toBe(false)
  expect(store.st(1).articleLoading).toBe(false)
  expect((await cancelled as Error).message).toBe('类型推断已取消')
  finishSave()
  await Bun.sleep(0)
  expect(send).not.toHaveBeenCalled()
  expect(ws.tasks.active).toHaveLength(0)
})

test('an old cancelled inference cannot unlock a replacement task', async () => {
  const store = useAnalysisStore()
  const first = store.inferStructure(session)
  const cancelled = first.catch((error: Error) => error)
  ws.tasks.active[0]!.cancel()
  const second = store.inferStructure(session)
  expect((await cancelled as Error).message).toBe('类型推断已取消')
  expect(session.ui.kindLocked).toBe(true)
  await Bun.sleep(0)
  replies.get('request-1')!({ id: 'request-1', type: 'analyze.articleResult', payload: { articleId: 1, paragraphs: [] } })
  await second
  expect(session.ui.kindLocked).toBe(false)
})

test('inference timeout releases the type lock and allows retry', async () => {
  const store = useAnalysisStore()
  const pending = store.inferStructure(session)
  const failed = pending.catch((error: Error) => error)
  await Bun.sleep(0)
  replies.get('request-0')!({ id: 'request-0', type: 'error', payload: { code: 'timeout' } })
  expect((await failed as Error).message).toContain('超时')
  expect(session.ui.kindLocked).toBe(false)
  expect(ws.tasks.items[0]?.status).toBe('failed')
  expect(replies.size).toBe(0)
})
