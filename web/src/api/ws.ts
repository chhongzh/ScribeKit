import { reactive } from 'vue'
import { wsErrorText } from '../core/labels'
import { BackgroundTasks } from './tasks'
import type { TaskKind } from './tasks'

// SPEC §5：单连接、JSON 文本帧、信封 {id, type, payload}。
// 断线指数退避重连；请求按 id 路由到发起者，服务端无 id 的推送走全局监听。

export interface Envelope<T = unknown> {
  id: string
  type: string
  payload: T
}

export type WsStatus = 'connecting' | 'open' | 'closed'

type Handler = (msg: Envelope) => void
type LogFn = (dir: 'out' | 'in' | 'sys', text: string, msg?: Envelope) => void

const MIN_DELAY = 500
const MAX_DELAY = 15000
const PING_EVERY = 25000
const TASK_TYPES: Record<string, { title: string; kind: TaskKind }> = {
  'analyze.paragraphs': { title: '段落分析', kind: 'paragraphs' },
  'analyze.article': { title: '全文分析', kind: 'article' },
  'analyze.text': { title: '建议文本分析', kind: 'text' },
  'scaffold.request': { title: '写作建议生成', kind: 'scaffold' },
}

export class WsClient {
  readonly tasks = new BackgroundTasks()
  readonly state = reactive({ status: 'closed' as WsStatus, retryIn: 0, attempts: 0, lastError: '', active: 0, usedTokens: 0, promptTokens: 0, completionTokens: 0, missingUsage: 0 })
  private sock: WebSocket | null = null
  private seq = 0
  private handlers = new Map<string, Handler>()
	private deadlines = new Map<string, ReturnType<typeof setTimeout>>()
  private listeners = new Map<string, Set<Handler>>()
  private retryTimer: ReturnType<typeof setTimeout> | null = null
  private countdown: ReturnType<typeof setInterval> | null = null
  private pingTimer: ReturnType<typeof setInterval> | null = null
  private stopped = false
  log: LogFn = () => {}

  constructor(private readonly url: string) {}

  start(): void {
    this.stopped = false
    this.connect()
  }

  stop(): void {
    this.stopped = true
    this.clearTimers()
    this.sock?.close()
  }

  /** 立即重连（状态栏“立即重连”）。 */
  reconnectNow(): void {
    this.clearTimers()
    this.state.attempts = 0
    if (this.sock && this.sock.readyState <= WebSocket.OPEN) return
    this.connect()
  }

  private clearTimers(): void {
    if (this.retryTimer) clearTimeout(this.retryTimer)
    if (this.countdown) clearInterval(this.countdown)
    this.retryTimer = null
    this.countdown = null
    this.state.retryIn = 0
  }

  private connect(): void {
    this.state.status = 'connecting'
    let sock: WebSocket
    try {
      sock = new WebSocket(this.url)
    } catch (e) {
      this.state.lastError = e instanceof Error ? e.message : String(e)
      this.scheduleRetry()
      return
    }
    this.sock = sock
    sock.onopen = () => {
      this.state.status = 'open'
      this.state.attempts = 0
      this.state.lastError = ''
      this.log('sys', '已连接 /api/ws')
      if (this.pingTimer) clearInterval(this.pingTimer)
      this.pingTimer = setInterval(() => this.send('ping', {}), PING_EVERY)
      this.emit({ id: '', type: '$open', payload: {} })
    }
    sock.onmessage = (ev) => {
      let msg: Envelope
      try {
        msg = JSON.parse(String(ev.data)) as Envelope
      } catch {
        this.log('sys', '收到无法解析的帧，已忽略')
        return
      }
      if (msg.type !== 'pong' && msg.type !== 'scaffold.delta' && msg.type !== 'agent.usage') this.log('in', `${msg.type}${msg.id ? ` #${msg.id}` : ''}`, msg)
      if (msg.id) this.deliver(msg)
      if (msg.type === 'agent.usage') {
        const usage = msg.payload as { activeAgents: number; totalTokens: number; promptTokens: number; completionTokens: number; missingUsage: number }
        this.state.active = usage.activeAgents
        this.state.usedTokens = usage.totalTokens
        this.state.promptTokens = usage.promptTokens
        this.state.completionTokens = usage.completionTokens
        this.state.missingUsage = usage.missingUsage
      }
      this.emit(msg)
    }
    sock.onclose = (ev) => {
      if (this.pingTimer) clearInterval(this.pingTimer)
      this.pingTimer = null
      this.sock = null
      this.state.status = 'closed'
      this.state.active = 0
      // 断开时所有进行中的请求都失败，发起者按 error 处理。
      for (const id of [...this.handlers.keys()]) {
        this.deliver({ id, type: 'error', payload: { code: 'disconnected', message: '连接已断开' } })
      }
      this.handlers.clear()
	  for (const timer of this.deadlines.values()) clearTimeout(timer)
	  this.deadlines.clear()
      this.log('sys', `连接断开${ev.code ? `（${ev.code}）` : ''}`)
      this.emit({ id: '', type: '$close', payload: {} })
      if (!this.stopped) this.scheduleRetry()
    }
    sock.onerror = () => {
      this.state.lastError = '无法连接 /api/ws'
    }
  }

  private scheduleRetry(): void {
    this.clearTimers()
    const delay = Math.min(MAX_DELAY, MIN_DELAY * 2 ** this.state.attempts)
    this.state.attempts++
    const until = Date.now() + delay
    this.state.retryIn = Math.ceil(delay / 1000)
    this.countdown = setInterval(() => {
      this.state.retryIn = Math.max(0, Math.ceil((until - Date.now()) / 1000))
    }, 250)
    this.retryTimer = setTimeout(() => {
      this.clearTimers()
      this.connect()
    }, delay)
  }

  get isOpen(): boolean {
    return this.sock?.readyState === WebSocket.OPEN
  }

  nextId(): string {
    this.seq++
    return `r-${Date.now().toString(36)}-${this.seq}`
  }

  /** 发送请求。未连接时返回 null，调用方负责提示。 */
  send(type: string, payload: unknown, handler?: Handler, id = this.nextId(), timeout = 190000): string | null {
    if (!this.sock || this.sock.readyState !== WebSocket.OPEN) {
      this.tasks.finish(id, 'failed', 'WebSocket 未连接，请重连后重试。')
      return null
    }
    if (TASK_TYPES[type]) {
      this.tasks.start({ id, articleId: (payload as { articleId: number }).articleId, ...TASK_TYPES[type], cancel: () => this.cancel(id) })
      this.tasks.running(id)
    }
    if (handler) {
      this.handlers.set(id, handler)
      this.deadlines.set(id, setTimeout(() => {
        this.send('cancel', { targetId: id })
        this.deliver({ id, type: 'error', payload: { code: 'timeout', message: '模型响应超时，请重试或检查模型服务。' } })
      }, timeout))
    }
    try {
      this.sock.send(JSON.stringify({ id, type, payload }))
    } catch {
      this.release(id)
      this.tasks.finish(id, 'failed', '请求发送失败，请重连后重试。')
      return null
    }
    if (type !== 'ping') this.log('out', `${type} #${id}`, { id, type, payload })
    return id
  }

  /** 等连接就绪，最多等 timeout 毫秒。页面刚打开时请求可能早于连接建立。 */
  whenOpen(timeout = 5000): Promise<boolean> {
    if (this.isOpen) return Promise.resolve(true)
    return new Promise((resolve) => {
      const off = this.on('$open', () => {
        clearTimeout(t)
        off()
        resolve(true)
      })
      const t = setTimeout(() => {
        off()
        resolve(false)
      }, timeout)
    })
  }

  release(id: string): void {
    this.handlers.delete(id)
    const timer = this.deadlines.get(id)
    if (timer) clearTimeout(timer)
    this.deadlines.delete(id)
  }

  cancel(targetId: string): void {
    if (this.handlers.has(targetId)) this.send('cancel', { targetId })
    this.deliver({ id: targetId, type: 'error', payload: { code: 'cancelled', message: '请求已取消' } })
  }

  private deliver(message: Envelope): void {
    const handler = this.handlers.get(message.id)
    if (message.type === 'error') {
      const payload = message.payload as { code?: string; message?: string }
      this.tasks.finish(message.id, payload.code === 'cancelled' ? 'cancelled' : 'failed', wsErrorText(payload.code ?? 'internal', payload.message))
      this.release(message.id)
    } else if (['analyze.done', 'analyze.articleResult', 'analyze.textResult', 'scaffold.done'].includes(message.type)) {
      this.tasks.finish(message.id, 'completed')
      this.release(message.id)
    }
    handler?.(message)
  }

  on(type: string, fn: Handler): () => void {
    let set = this.listeners.get(type)
    if (!set) {
      set = new Set()
      this.listeners.set(type, set)
    }
    set.add(fn)
    return () => set.delete(fn)
  }

  private emit(msg: Envelope): void {
    this.listeners.get(msg.type)?.forEach((fn) => fn(msg))
  }
}

function wsUrl(): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${location.host}/api/ws`
}

export const ws = new WsClient(wsUrl())
