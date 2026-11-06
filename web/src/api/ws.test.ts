import { afterEach, expect, test } from 'bun:test'

Object.defineProperty(globalThis, 'location', { value: { protocol: 'http:', host: 'localhost' }, configurable: true })
const { WsClient } = await import('./ws')
const originalWebSocket = globalThis.WebSocket

class FakeSocket {
  static OPEN = 1
  readyState = 1
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: ((event: { code: number }) => void) | null = null
  onerror: (() => void) | null = null
  sent: string[] = []
  static latest: FakeSocket

  constructor() { FakeSocket.latest = this }
  send(message: string) { this.sent.push(message) }
  close() { this.onclose?.({ code: 1000 }) }
}

afterEach(() => { globalThis.WebSocket = originalWebSocket })

test('unanswered requests expire and send cancellation', async () => {
  globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
  const client = new WsClient('ws://test')
  client.start()
  const message = await new Promise<{ type: string; payload: unknown }>((resolve) => {
    client.send('analyze.article', {}, resolve, 'deadline-test', 5)
  })
  expect(message.type).toBe('error')
  expect(message.payload).toEqual({ code: 'timeout', message: '模型响应超时，请重试或检查模型服务。' })
  expect(FakeSocket.latest.sent.map((frame) => JSON.parse(frame).type)).toEqual(['analyze.article', 'cancel'])
  expect(client.tasks.items[0]?.status).toBe('failed')
  expect(client.tasks.active).toHaveLength(0)
  client.stop()
})

test('release clears the request deadline', async () => {
  globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
  const client = new WsClient('ws://test')
  client.start()
  let replies = 0
  client.send('analyze.article', {}, () => { replies++ }, 'completed', 5)
  client.release('completed')
  await Bun.sleep(20)
  expect(replies).toBe(0)
  expect(FakeSocket.latest.sent).toHaveLength(1)
  client.stop()
})

test('agent usage snapshots replace totals and do not accumulate repeated frames', () => {
  globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
  const client = new WsClient('ws://test')
  client.start()
  const snapshot = { activeAgents: 2, totalTokens: 16, promptTokens: 10, completionTokens: 6, missingUsage: 0 }
  const receive = (payload: typeof snapshot) => FakeSocket.latest.onmessage?.({ data: JSON.stringify({ id: '', type: 'agent.usage', payload }) })
  receive(snapshot)
  receive(snapshot)
  expect(client.state.active).toBe(2)
  expect(client.state.usedTokens).toBe(16)
  receive({ ...snapshot, activeAgents: 0, totalTokens: 24, promptTokens: 15, completionTokens: 9 })
  expect(client.state.active).toBe(0)
  expect(client.state.usedTokens).toBe(24)
  client.stop()
})

test('disconnect clears active agents even without request handlers', () => {
  globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
  const client = new WsClient('ws://test')
  client.start()
  FakeSocket.latest.onmessage?.({ data: JSON.stringify({ id: '', type: 'agent.usage', payload: { activeAgents: 3, totalTokens: 8, promptTokens: 5, completionTokens: 3, missingUsage: 0 } }) })
  expect(client.state.active).toBe(3)
  client.stop()
  expect(client.state.active).toBe(0)
  expect(client.state.usedTokens).toBe(8)
})

test('cancelling one background task settles its caller and ignores late results without affecting another', () => {
  globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
  const client = new WsClient('ws://test')
  client.start()
  const first: unknown[] = []
  const second: unknown[] = []
  client.send('analyze.article', { articleId: 1 }, (message) => first.push(message), 'first')
  client.send('scaffold.request', { articleId: 2 }, (message) => second.push(message), 'second')
  expect(client.tasks.active).toHaveLength(2)
  client.tasks.active.find((task) => task.id === 'first')!.cancel()
  expect(first).toEqual([{ id: 'first', type: 'error', payload: { code: 'cancelled', message: '请求已取消' } }])
  expect(client.tasks.active.map((task) => task.id)).toEqual(['second'])
  FakeSocket.latest.onmessage?.({ data: JSON.stringify({ id: 'first', type: 'analyze.articleResult', payload: {} }) })
  expect(first).toHaveLength(1)
  FakeSocket.latest.onmessage?.({ data: JSON.stringify({ id: 'second', type: 'scaffold.done', payload: {} }) })
  expect(second).toHaveLength(1)
  expect(client.tasks.active).toHaveLength(0)
  expect(client.tasks.items.find((task) => task.id === 'first')?.status).toBe('cancelled')
  client.stop()
})

test('disconnect fails all running tasks and clearing history preserves active work', () => {
  globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
  const client = new WsClient('ws://test')
  client.start()
  client.send('analyze.text', { articleId: 1 }, () => {}, 'finished')
  FakeSocket.latest.onmessage?.({ data: JSON.stringify({ id: 'finished', type: 'analyze.textResult', payload: {} }) })
  client.send('analyze.article', { articleId: 2 }, () => {}, 'running')
  client.tasks.clearFinished()
  expect(client.tasks.items.map((task) => task.id)).toEqual(['running'])
  client.stop()
  expect(client.tasks.active).toHaveLength(0)
  expect(client.tasks.items[0]?.status).toBe('failed')
  expect(client.tasks.items[0]?.error).toContain('连接')
})

test('closing an article cancels only its tasks and removes stale result actions', () => {
  globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
  const client = new WsClient('ws://test')
  client.start()
  client.send('analyze.article', { articleId: 1 }, () => {}, 'article-one')
  client.send('scaffold.request', { articleId: 1 }, () => {}, 'generation-one')
  client.send('analyze.text', { articleId: 2 }, () => {}, 'article-two')
  client.tasks.items.find((task) => task.id === 'article-one')!.open = () => {}
  client.tasks.cancelArticle(1)
  expect(client.tasks.active.map((task) => task.id)).toEqual(['article-two'])
  expect(client.tasks.items.filter((task) => task.articleId === 1).every((task) => task.status === 'cancelled' && !task.open)).toBe(true)
  expect(FakeSocket.latest.sent.map((message) => JSON.parse(message)).filter((message) => message.type === 'cancel').map((message) => message.payload.targetId).sort()).toEqual(['article-one', 'generation-one'])
  client.stop()
})

test('history limits keep the newest finished records and preserve older active tasks', () => {
  globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket
  const client = new WsClient('ws://test')
  client.start()
  client.send('analyze.article', { articleId: 1 }, () => {}, 'still-running')
  for (let index = 0; index < 105; index++) {
    const id = `finished-${index}`
    client.send('analyze.text', { articleId: 2 }, () => {}, id)
    FakeSocket.latest.onmessage?.({ data: JSON.stringify({ id, type: 'analyze.textResult', payload: {} }) })
  }
  expect(client.tasks.items).toHaveLength(101)
  expect(client.tasks.items[0]?.id).toBe('finished-104')
  expect(client.tasks.items.some((task) => task.id === 'finished-4')).toBe(false)
  expect(client.tasks.active.map((task) => task.id)).toEqual(['still-running'])
  client.stop()
})
