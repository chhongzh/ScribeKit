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
  const log = useLogStore()

  function st(articleId: number): ArticleAnalysisState {
    if (!status[articleId]) status[articleId] = { inflight: 0, error: '', errorCode: '', articleLoading: false }
    return status[articleId]!
  }

  function attach(session: ArticleSession) {
    sessionOf.set(session.articleId, session)
    session.analysisLookup = (id) => byParagraph[id]
  }

  function detach(articleId: number) {
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
    try {
      await session.sync.flush()
    } catch {
      s.error = '有改动尚未保存，先处理保存失败再分析。'
      return
    }
    const ids = keys
      .map((k) => session.sync.idOf(k))
      .filter((id): id is number => id !== null)
      .filter((id) => {
        const text = session.textOfId(id) ?? ''
        if (text.trim() === '') return false
        const old = byParagraph[id]
        return !old || old.textHash !== textHash(text)
      })
    if (ids.length === 0) return
    await ws.whenOpen()
    const remaining = new Set(ids)
    s.inflight++
    s.error = ''
    const done = () => {
      s.inflight = Math.max(0, s.inflight - 1)
      ws.release(reqId!)
    }
    const reqId = ws.send('analyze.paragraphs', { articleId: session.articleId, paragraphIds: ids }, (msg: Envelope) => {
      if (msg.type === 'analyze.result') {
        const a = msg.payload as ParagraphAnalysis
        accept(a)
        remaining.delete(a.paragraphId)
        if (remaining.size === 0) done()
      } else if (msg.type === 'error') {
        fail(session.articleId, msg.payload as { code?: string; message?: string })
        done()
      }
    })
    if (!reqId) {
      s.inflight = Math.max(0, s.inflight - 1)
      s.error = 'WebSocket 未连接，分析请求没有发出。'
    }
  }

  async function analyzeArticle(session: ArticleSession) {
    const s = st(session.articleId)
    await analyzeParagraphs(
      session,
      session.localRows().map((r) => r.key),
    )
    s.articleLoading = true
    await ws.whenOpen()
    const id = ws.send('analyze.article', { articleId: session.articleId }, (msg) => {
      if (msg.type === 'analyze.articleResult') {
        byArticle[session.articleId] = msg.payload as ArticleAnalysis
        s.articleLoading = false
        ws.release(id!)
      } else if (msg.type === 'error') {
        fail(session.articleId, msg.payload as { code?: string; message?: string })
        s.articleLoading = false
        ws.release(id!)
      }
    })
    if (!id) {
      s.articleLoading = false
      s.error = 'WebSocket 未连接，分析请求没有发出。'
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
      const id = ws.send('analyze.text', { articleId, paragraphKind, text }, (msg) => {
        if (msg.type === 'analyze.textResult') {
          ws.release(id!)
          resolve(msg.payload as ParagraphAnalysis)
        } else if (msg.type === 'error') {
          ws.release(id!)
          const p = msg.payload as { code?: string; message?: string }
          reject(new Error(p.code === 'disconnected' ? '连接断开' : wsErrorText(p.code ?? 'internal', p.message)))
        }
      })
      if (!id) reject(new Error('WebSocket 未连接'))
    })
  }

  function resume() {
    paused.value = false
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
    schedule,
    analyzeText,
    resume,
  }
})
