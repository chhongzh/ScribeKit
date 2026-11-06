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
