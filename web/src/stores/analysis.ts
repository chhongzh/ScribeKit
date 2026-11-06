import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import type { ArticleAnalysis, ParagraphAnalysis, ParagraphKind } from '../api/types'
import { ws, type Envelope } from '../api/ws'
import { textHash } from '../core/hash'
import { wsErrorText } from '../core/labels'
import type { ArticleSession } from '../editor/session'
import { useLogStore } from './log'
import { useSettingsStore } from './settings'

const IDLE_MS = 1200

interface ArticleAnalysisState {
  inflight: number
  preparing: number
  error: string
  errorCode: string
  articleLoading: boolean
}

/** Analyzer 结果。只读：只存、只比对 textHash，不改正文。 */
export const useAnalysisStore = defineStore('analysis', () => {
  const byParagraph = reactive<Record<number, ParagraphAnalysis>>({})
  const byArticle = reactive<Record<number, ArticleAnalysis>>({})
  const status = reactive<Record<number, ArticleAnalysisState>>({})
  /** 未配置模型时暂停自动分析，避免每次停笔都报同一个错。 */
  const paused = ref(false)
  const timers = new Map<string, ReturnType<typeof setTimeout>>()
  const sessionOf = new Map<number, ArticleSession>()
  const requests = new Map<string, { articleId: number; done: () => void }>()
  const epochs = new Map<number, number>()
  const pendingParagraphs = new Set<string>()
  const articleTasks = new Map<number, string>()
  const log = useLogStore()

  function st(articleId: number): ArticleAnalysisState {
    if (!status[articleId]) status[articleId] = { inflight: 0, preparing: 0, error: '', errorCode: '', articleLoading: false }
    return status[articleId]!
  }

  function attach(session: ArticleSession) {
    sessionOf.set(session.articleId, session)
    session.analysisLookup = (id) => byParagraph[id]
  }

  function detach(articleId: number) {
    cancel(articleId)
    sessionOf.delete(articleId)
    for (const [k, t] of timers) {
      if (k.startsWith(`${articleId}:`)) {
        clearTimeout(t)
        timers.delete(k)
      }
    }
  }

  function fail(articleId: number, payload: { code?: string; message?: string }) {
    const s = st(articleId)
    if (payload.code === 'cancelled') {
      s.errorCode = ''
      s.error = ''
      return
    }
    s.errorCode = payload.code ?? 'internal'
    s.error = payload.code === 'disconnected' ? '连接断开，分析中止。' : wsErrorText(s.errorCode, payload.message)
    if (payload.code === 'model_not_configured') paused.value = true
  }

  function accept(a: ParagraphAnalysis) {
    for (const session of sessionOf.values()) {
      const text = session.textOfId(a.paragraphId)
      if (text === null) continue
      if (textHash(text) !== a.textHash) {
        log.add('sys', `第 ${a.paragraphId} 号段落的分析结果对应旧文本，已丢弃`)
        return
      }
      byParagraph[a.paragraphId] = a
      session.refresh()
      return
    }
  }

  /** 把段落交给 Analyzer。调用前会先把这些段落写入后端，后端按库里的文本分析。 */
  async function analyzeParagraphs(session: ArticleSession, keys: string[]) {
    const s = st(session.articleId)
    const epoch = epochs.get(session.articleId) ?? 0
    const current = () => sessionOf.get(session.articleId) === session && (epochs.get(session.articleId) ?? 0) === epoch
    const taskId = ws.nextId()
    let preparing = true
    const finishPreparing = () => {
      if (!preparing) return
      preparing = false
      s.preparing = Math.max(0, s.preparing - 1)
    }
    const task = ws.tasks.start({
      id: taskId, articleId: session.articleId, title: '段落分析', kind: 'paragraphs',
      cancel: () => { ws.cancel(taskId); finishPreparing() },
    })
    s.preparing++
    try {
      await session.sync.flush()
      await ws.whenOpen()
    } catch {
      if (task.status !== 'cancelled' && current()) s.error = '有改动尚未保存，先处理保存失败再分析。'
      ws.tasks.finish(taskId, 'failed', '有改动尚未保存，先处理保存失败再分析。')
      return
    } finally {
      finishPreparing()
    }
    if (!current() || task.status === 'cancelled') { ws.tasks.finish(taskId, 'cancelled'); return }
    const fingerprint = (id: number) => `${id}:${textHash(session.textOfId(id) ?? '')}`
    const ids = [...new Set(keys
      .map((k) => session.sync.idOf(k))
      .filter((id): id is number => id !== null))]
      .filter((id) => {
        if (pendingParagraphs.has(fingerprint(id))) return false
        const text = session.textOfId(id) ?? ''
        if (text.trim() === '') return false
        const old = byParagraph[id]
        return !old || old.partial || old.textHash !== textHash(text)
      })
    if (ids.length === 0) { ws.tasks.finish(taskId, 'completed'); return }
    const fingerprints = ids.map(fingerprint)
    for (const key of fingerprints) pendingParagraphs.add(key)
    s.inflight++
    s.error = ''
    s.errorCode = ''
    let finished = false
    const done = () => {
      if (finished) return
      finished = true
      s.inflight = Math.max(0, s.inflight - 1)
      ws.release(reqId!)
      requests.delete(reqId!)
      for (const key of fingerprints) pendingParagraphs.delete(key)
    }
    const reqId = ws.send('analyze.paragraphs', { articleId: session.articleId, paragraphIds: ids }, (msg: Envelope) => {
      if (msg.type === 'analyze.result') {
        const a = msg.payload as ParagraphAnalysis
        accept(a)
      } else if (msg.type === 'analyze.done') {
        ws.tasks.finish(taskId, 'completed')
        done()
      } else if (msg.type === 'error') {
        const payload = msg.payload as { code?: string; message?: string }
        ws.tasks.finish(taskId, payload.code === 'cancelled' ? 'cancelled' : 'failed', wsErrorText(payload.code ?? 'internal', payload.message))
        fail(session.articleId, msg.payload as { code?: string; message?: string })
        done()
      }
    }, taskId)
    if (!reqId) {
      done()
      s.error = 'WebSocket 未连接，分析请求没有发出。'
      ws.tasks.finish(taskId, 'failed', s.error)
    } else {
      requests.set(reqId, { articleId: session.articleId, done })
    }
  }

  async function analyzeArticle(session: ArticleSession) {
    const s = st(session.articleId)
    if (s.articleLoading) return
    s.articleLoading = true
    s.error = ''
    s.errorCode = ''
    const epoch = epochs.get(session.articleId) ?? 0
    const id = ws.nextId()
    articleTasks.set(session.articleId, id)
    const unlock = () => {
      if (articleTasks.get(session.articleId) !== id) return
      articleTasks.delete(session.articleId)
      s.articleLoading = false
    }
    const task = ws.tasks.start({ id, articleId: session.articleId, title: '全文分析', kind: 'article', cancel: () => { ws.cancel(id); unlock() } })
    await analyzeParagraphs(
      session,
      session.localRows().map((r) => r.key),
    )
    if (task.status === 'cancelled' || articleTasks.get(session.articleId) !== id) return
    if (!await ws.whenOpen()) {
      unlock()
      s.error = 'WebSocket 未连接，分析请求没有发出。'
      ws.tasks.finish(id, 'failed', s.error)
      return
    }
    if (articleTasks.get(session.articleId) !== id || sessionOf.get(session.articleId) !== session || (epochs.get(session.articleId) ?? 0) !== epoch || s.error) {
      unlock()
      ws.tasks.finish(id, s.error ? 'failed' : 'cancelled', s.error)
      return
    }
    s.error = ''
    s.errorCode = ''
    const done = () => {
      unlock()
      ws.release(id!)
      requests.delete(id!)
    }
    const sent = ws.send('analyze.article', { articleId: session.articleId }, (msg) => {
      if (msg.type === 'analyze.articleResult') {
        byArticle[session.articleId] = msg.payload as ArticleAnalysis
        done()
      } else if (msg.type === 'error') {
        fail(session.articleId, msg.payload as { code?: string; message?: string })
        done()
      }
    }, id)
    if (!sent) {
      unlock()
      s.error = 'WebSocket 未连接，分析请求没有发出。'
      ws.tasks.finish(id, 'failed', s.error)
    } else {
      requests.set(id, { articleId: session.articleId, done })
    }
  }

  async function inferStructure(session: ArticleSession, open?: () => void): Promise<ArticleAnalysis> {
    const state = st(session.articleId)
    if (state.articleLoading) throw new Error('全文分析正在进行')
    const epoch = epochs.get(session.articleId) ?? 0
    const id = ws.nextId()
    articleTasks.set(session.articleId, id)
    session.ui.kindLocked = true
    state.articleLoading = true
    state.error = ''
    state.errorCode = ''
    const unlock = () => {
      if (articleTasks.get(session.articleId) !== id) return
      articleTasks.delete(session.articleId)
      session.ui.kindLocked = false
      state.articleLoading = false
    }
    let rejectCancelled!: (error: Error) => void
    const cancelled = new Promise<never>((_resolve, reject) => { rejectCancelled = reject })
    const task = ws.tasks.start({
      id, articleId: session.articleId, title: '段落类型推断', kind: 'inference', open,
      cancel: () => {
        ws.cancel(id)
        ws.tasks.finish(id, 'cancelled')
        unlock()
        rejectCancelled(new Error('类型推断已取消'))
      },
    })
    try {
      await Promise.race([session.sync.flush(), cancelled])
      if (!await Promise.race([ws.whenOpen(), cancelled])) throw new Error('WebSocket 未连接，请重连后重试')
      if (task.status === 'cancelled' || sessionOf.get(session.articleId) !== session || (epochs.get(session.articleId) ?? 0) !== epoch) throw new Error('类型推断已取消')
      return await Promise.race([new Promise<ArticleAnalysis>((resolve, reject) => {
        const cleanup = () => { ws.release(id!); requests.delete(id!) }
        const sent = ws.send('analyze.article', { articleId: session.articleId }, (message) => {
          if (message.type === 'analyze.articleResult') {
            const result = message.payload as ArticleAnalysis
            byArticle[session.articleId] = result
            ws.tasks.finish(id, 'completed')
            cleanup()
            resolve(result)
          } else if (message.type === 'error') {
            fail(session.articleId, message.payload as { code?: string; message?: string })
            cleanup()
            reject(new Error(state.error || '类型推断已取消'))
          }
        }, id)
        if (!sent) reject(new Error('WebSocket 未连接'))
        else requests.set(id, { articleId: session.articleId, done: () => { cleanup(); reject(new Error('分析已取消')) } })
      }), cancelled])
    } catch (error) {
      ws.tasks.finish(id, 'failed', error instanceof Error ? error.message : String(error))
      throw error
    } finally {
      ws.release(id)
      requests.delete(id)
      unlock()
    }
  }

  /** 停止输入 1.2 秒后自动分析（Settings.autoAnalyze）。 */
  function schedule(session: ArticleSession, keys: string[]) {
    const settings = useSettingsStore()
    if (!settings.settings?.autoAnalyze || paused.value) return
    for (const key of keys) {
      const tk = `${session.articleId}:${key}`
      const old = timers.get(tk)
      if (old) clearTimeout(old)
      timers.set(
        tk,
        setTimeout(() => {
          timers.delete(tk)
          if (sessionOf.get(session.articleId) === session) void analyzeParagraphs(session, [key])
        }, IDLE_MS),
      )
    }
  }

  /** 分析一段尚未入库的文本（建议评审用）。 */
  async function analyzeText(articleId: number, paragraphKind: ParagraphKind, text: string): Promise<ParagraphAnalysis> {
    await ws.whenOpen()
    return new Promise((resolve, reject) => {
      const cleanup = () => {
        ws.release(id!)
        requests.delete(id!)
      }
      const id = ws.send('analyze.text', { articleId, paragraphKind, text }, (msg) => {
        if (msg.type === 'analyze.textResult') {
          cleanup()
          resolve(msg.payload as ParagraphAnalysis)
        } else if (msg.type === 'error') {
          cleanup()
          const p = msg.payload as { code?: string; message?: string }
          reject(new Error(p.code === 'disconnected' ? '连接断开' : wsErrorText(p.code ?? 'internal', p.message)))
        }
      })
      if (!id) reject(new Error('WebSocket 未连接'))
      else requests.set(id, { articleId, done: () => {
        cleanup()
        reject(new Error('分析已取消'))
      } })
    })
  }

  function resume() {
    paused.value = false
  }

  function cancel(articleId: number) {
    epochs.set(articleId, (epochs.get(articleId) ?? 0) + 1)
    const articleTaskId = articleTasks.get(articleId)
    if (articleTaskId) ws.tasks.active.find((task) => task.id === articleTaskId)?.cancel()
    for (const [key, timer] of timers) {
      if (!key.startsWith(`${articleId}:`)) continue
      clearTimeout(timer)
      timers.delete(key)
    }
    for (const [id, request] of requests) {
      if (request.articleId !== articleId) continue
      ws.cancel(id)
      request.done()
      ws.tasks.finish(id, 'cancelled')
    }
    for (const task of ws.tasks.active.filter((item) => item.articleId === articleId && item.kind === 'paragraphs')) task.cancel()
    st(articleId).articleLoading = false
    st(articleId).preparing = 0
  }

  return {
    byParagraph,
    byArticle,
    status,
    paused,
    st,
    attach,
    detach,
    analyzeParagraphs,
    analyzeArticle,
    inferStructure,
    schedule,
    analyzeText,
    resume,
    cancel,
  }
})
