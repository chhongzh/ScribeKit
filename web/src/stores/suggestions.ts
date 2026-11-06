import { defineStore } from 'pinia'
import { reactive } from 'vue'
import { api, describeError } from '../api/http'
import type { ParagraphAnalysis, ParagraphKind, ScaffoldRequest, Suggestion, SuggestionHalf } from '../api/types'
import { ws } from '../api/ws'
import { isUserOwnedKind, wsErrorText } from '../core/labels'
import { cpLength } from '../core/offsets'
import { finalText } from '../core/review'
import type { WidgetSpec, TargetSpec } from '../editor/decorations'
import type { ArticleSession } from '../editor/session'
import { useAnalysisStore } from './analysis'
import { useEditorStore } from './editor'
import { useNotifyStore } from './notify'
import { useUiStore } from './ui'

export interface Stream {
  requestId: string
  articleId: number
  suggestionId: number | null
  anchorKey: string
  anchorCp: number
  block: boolean
  title: string
  raw: string
  text: string
  request: ScaffoldRequest
  paragraphKind: ParagraphKind
}

/** 编辑器侧组装的请求草稿：段落用本地 key，发送前换成服务器 id。偏移是码点。 */
export interface ScaffoldDraft {
  cursor: { key: string; offset: number } | null
  selection: { key: string; start: number; end: number } | null
  commandRange: { key: string; start: number; end: number } | null
  instruction: string
  mode: ScaffoldRequest['mode']
  newParagraphKind: ParagraphKind | null
  force: boolean
}

export interface StreamView {
  title: string
  anchorKey: string
  anchorCp: number
  block: boolean
  paragraphKind: ParagraphKind
}

interface TextAnalysisState {
  status: 'loading' | 'ok' | 'error'
  data?: ParagraphAnalysis
  error?: string
}

/** 请求发出时目标段落的文本，用来在合入前发现“请求之后这段又被改过”。 */
interface Snapshot {
  key: string
  text: string
}

function cleanPreview(raw: string): string {
  return raw.replace(/<\/?rationale>/g, '').replace(/<\/?[a-z]*$/i, '')
}

export const useSuggestionStore = defineStore('suggestions', () => {
  const byArticle = reactive<Record<number, Suggestion[]>>({})
  const loadState = reactive<Record<number, { loading: boolean; error: string }>>({})
  const streams = reactive<Stream[]>([])
  const textAnalysis = reactive<Record<number, TextAnalysisState>>({})
  /** 转折段 / Call Back 段被后端降级为建议时，记下原请求以便“仍然生成”。 */
  const degraded = reactive<Record<number, ScaffoldRequest>>({})
  const busy = reactive<Record<number, string>>({})
  const snapshots = new Map<string, Snapshot>()
  const patchTimers = new Map<number, ReturnType<typeof setTimeout>>()
  const sessions = new Map<number, ArticleSession>()

  const notify = useNotifyStore()
  const ui = useUiStore()
  const analysis = useAnalysisStore()

  function attach(session: ArticleSession) {
    sessions.set(session.articleId, session)
  }
  function detach(articleId: number) {
    sessions.delete(articleId)
  }

  function list(articleId: number): Suggestion[] {
    return byArticle[articleId] ?? []
  }

  function find(id: number): Suggestion | undefined {
    for (const l of Object.values(byArticle)) {
      const s = l.find((x) => x.id === id)
      if (s) return s
    }
    return undefined
  }

  async function load(articleId: number) {
    loadState[articleId] = { loading: true, error: '' }
    try {
      const items = await api.listSuggestions(articleId, 'pending')
      byArticle[articleId] = items
      loadState[articleId] = { loading: false, error: '' }
      for (const s of items) if (s.op !== 'advice' && s.text) void analyzeSuggestion(s)
    } catch (e) {
      loadState[articleId] = { loading: false, error: describeError(e) }
    }
  }

  async function analyzeSuggestion(s: Suggestion) {
    textAnalysis[s.id] = { status: 'loading' }
    try {
      const data = await analysis.analyzeText(s.articleId, s.paragraphKind, s.text)
      textAnalysis[s.id] = { status: 'ok', data }
    } catch (e) {
      textAnalysis[s.id] = { status: 'error', error: e instanceof Error ? e.message : String(e) }
    }
  }

  function upsert(s: Suggestion) {
    const l = byArticle[s.articleId] ?? (byArticle[s.articleId] = [])
    const i = l.findIndex((x) => x.id === s.id)
    if (i >= 0) l.splice(i, 1, s)
    else l.unshift(s)
  }

  function remove(s: Suggestion) {
    const l = byArticle[s.articleId]
    if (l) {
      const i = l.findIndex((x) => x.id === s.id)
      if (i >= 0) l.splice(i, 1)
    }
    delete degraded[s.id]
    delete textAnalysis[s.id]
    if (ui.reviewing === s.id) ui.reviewing = null
    refreshEditor(s.articleId)
  }

  function refreshEditor(articleId: number) {
    const session = sessions.get(articleId)
    if (!session) return
    session.widgets = widgetsFor(articleId, session)
    session.targets = targetsFor(articleId, session)
    session.refresh()
  }

  /**
   * 发起 Scaffolder 请求。调用前编辑器里的改动会先写入后端（后端按库里的文本取上下文）。
   * anchor 是流式文字在编辑器里显示的位置。
   */
  async function request(session: ArticleSession, draft: ScaffoldDraft, view: StreamView): Promise<boolean> {
    try {
      await session.sync.flush()
    } catch {
      notify.error('有改动没有保存成功，先在编辑器顶部处理保存失败，再交给 Scaffolder。')
      return false
    }
    // flush 之后新段落才有 id；草稿里用的是本地 key，这里换成服务器 id。
    const id = (key: string) => session.sync.idOf(key)
    const missing = [draft.cursor?.key, draft.selection?.key, draft.commandRange?.key].some(
      (k) => k !== undefined && id(k) === null,
    )
    if (missing) {
      notify.error('光标所在的段落还没有保存到后端，稍后再试。')
      return false
    }
    const full: ScaffoldRequest = {
      articleId: session.articleId,
      cursor: draft.cursor ? { paragraphId: id(draft.cursor.key)!, offset: draft.cursor.offset } : null,
      selection: draft.selection
        ? { paragraphId: id(draft.selection.key)!, start: draft.selection.start, end: draft.selection.end }
        : null,
      commandRange: draft.commandRange
        ? { paragraphId: id(draft.commandRange.key)!, start: draft.commandRange.start, end: draft.commandRange.end }
        : null,
      instruction: draft.instruction,
      mode: draft.mode,
      newParagraphKind: draft.newParagraphKind,
      force: draft.force,
    }
    await ws.whenOpen()
    return send(session, full, view)
  }

  function send(session: ArticleSession, full: ScaffoldRequest, view: StreamView): boolean {
    const requestId = ws.nextId()
    const stream: Stream = {
      requestId,
      articleId: session.articleId,
      suggestionId: null,
      anchorKey: view.anchorKey,
      anchorCp: view.anchorCp,
      block: view.block,
      title: view.title,
      raw: '',
      text: '',
      request: full,
      paragraphKind: view.paragraphKind,
    }
    const snapText = session.textOfKey(view.anchorKey)
    if (snapText !== null) snapshots.set(requestId, { key: view.anchorKey, text: snapText })

    const sent = ws.send(
      'scaffold.request',
      full,
      (msg) => {
        const st = streams.find((x) => x.requestId === requestId)
        switch (msg.type) {
          case 'scaffold.started':
            if (st) st.suggestionId = (msg.payload as { suggestionId: number }).suggestionId
            break
          case 'scaffold.delta':
            // delta 只是预览，最终文本以 scaffold.done 为准。advise 的原始增量可能带 <rationale> 标签，显示时去掉。
            if (st) {
              st.raw += (msg.payload as { text: string }).text
              st.text = cleanPreview(st.raw)
              refreshEditor(session.articleId)
            }
            break
          case 'scaffold.done': {
            ws.release(requestId)
            dropStream(requestId)
            const s = msg.payload as Suggestion
            upsert(s)
            if (s.op === 'advice' && full.mode !== 'advise' && !full.force && isUserOwnedKind(view.paragraphKind)) {
              degraded[s.id] = full
            }
            if (s.op !== 'advice' && s.text) void analyzeSuggestion(s)
            ui.review(s.id)
            refreshEditor(session.articleId)
            break
          }
          case 'error': {
            ws.release(requestId)
            dropStream(requestId)
            refreshEditor(session.articleId)
            const p = msg.payload as { code?: string; message?: string }
            reportError(p.code ?? 'internal', p.message)
            break
          }
        }
      },
      requestId,
    )
    if (!sent) {
      notify.error('WebSocket 未连接，请求没有发出。状态栏会显示重连进度。')
      return false
    }
    streams.push(stream)
    refreshEditor(session.articleId)
    return true
  }

  function reportError(code: string, message?: string) {
    if (code === 'cancelled') {
      notify.push('已取消生成。')
      return
    }
    if (code === 'disconnected') {
      notify.error('连接断开，生成中止。重连后可以再试一次。')
      return
    }
    if (code === 'model_not_configured') {
      notify.error(wsErrorText(code), { label: '打开设置', run: () => useEditorStore().openSettings() })
      return
    }
    notify.error(wsErrorText(code, message))
  }

  function dropStream(requestId: string) {
    const i = streams.findIndex((x) => x.requestId === requestId)
    if (i >= 0) streams.splice(i, 1)
  }

  /** 取消正在生成的建议（Esc）。返回是否有可取消的请求。 */
  function cancelLatest(articleId: number): boolean {
    const st = [...streams].reverse().find((x) => x.articleId === articleId)
    if (!st) return false
    cancel(st.requestId)
    return true
  }

  function cancel(requestId: string) {
    const st = streams.find((x) => x.requestId === requestId)
    ws.cancel(requestId)
    ws.release(requestId)
    dropStream(requestId)
    if (st) refreshEditor(st.articleId)
    notify.push('已取消生成。')
  }

  // ---------- 评审 ----------

  function decide(s: Suggestion, index: number, state: SuggestionHalf['state'], edited?: string) {
    const h = s.halves.find((x) => x.index === index)
    if (!h) return
    h.state = state
    h.edited = state === 'edited' ? (edited ?? h.edited ?? h.text) : null
    schedulePatch(s)
  }

  function decideAll(s: Suggestion, state: 'accepted' | 'rejected') {
    for (const h of s.halves) if (h.state === 'pending') h.state = state
    schedulePatch(s)
  }

  function schedulePatch(s: Suggestion) {
    const old = patchTimers.get(s.id)
    if (old) clearTimeout(old)
    patchTimers.set(
      s.id,
      setTimeout(() => {
        patchTimers.delete(s.id)
        api.updateSuggestion(s.id, { halves: s.halves }).catch((e) => {
          notify.error(`评审进度没有保存：${describeError(e)}。刷新页面后这些决定会丢失。`)
        })
      }, 400),
    )
  }

  /** 合入前的检查：返回需要用户确认的原因，没有问题返回 null。 */
  function applyWarning(s: Suggestion): string | null {
    if (s.op !== 'replace' || !s.target || !('paragraphId' in s.target)) return null
    const session = sessions.get(s.articleId)
    if (!session) return null
    const key = session.sync.keyOf(s.target.paragraphId)
    if (!key) return '目标段落已经不在文章里了，后端可能拒绝合入。'
    const text = session.textOfKey(key) ?? ''
    if (s.target.end > cpLength(text)) return '目标段落在请求之后变短了，替换位置已经不准确。'
    const snap = snapshots.get(s.requestId)
    if (snap && snap.text !== text) return '目标段落在请求之后被改过，替换位置可能偏移。'
    return null
  }

  async function apply(s: Suggestion) {
    const session = sessions.get(s.articleId)
    const text = finalText(s.halves)
    busy[s.id] = '正在合入'
    try {
      if (session) await session.sync.flush()
      const pt = patchTimers.get(s.id)
      if (pt) {
        clearTimeout(pt)
        patchTimers.delete(s.id)
        await api.updateSuggestion(s.id, { halves: s.halves })
      }
      const res = await api.applySuggestion(s.id, text)
      const after = s.op === 'insert_paragraph' && s.target && 'afterParagraphId' in s.target ? s.target.afterParagraphId : undefined
      session?.applyServerParagraphs(res.paragraphs, after)
      snapshots.delete(s.requestId)
      remove(s)
      notify.push(s.op === 'insert_paragraph' ? '已作为新段落合入。' : '已合入正文。')
    } catch (e) {
      notify.error(`合入失败：${describeError(e)}`)
    } finally {
      delete busy[s.id]
    }
  }

  async function reject(s: Suggestion) {
    busy[s.id] = '正在拒绝'
    try {
      await api.rejectSuggestion(s.id)
      snapshots.delete(s.requestId)
      remove(s)
    } catch (e) {
      notify.error(`操作失败：${describeError(e)}`)
    } finally {
      delete busy[s.id]
    }
  }

  /** “仍然生成”：同一请求带 force 重发。 */
  async function force(s: Suggestion) {
    const req = degraded[s.id]
    const session = sessions.get(s.articleId)
    if (!req || !session) return
    const snap = snapshots.get(s.requestId)
    const anchorKey =
      (req.cursor && session.sync.keyOf(req.cursor.paragraphId)) ?? snap?.key ?? session.localRows()[0]?.key ?? ''
    delete degraded[s.id]
    try {
      await session.sync.flush()
    } catch {
      notify.error('有改动没有保存成功，先在编辑器顶部处理保存失败，再交给 Scaffolder。')
      return
    }
    await ws.whenOpen()
    send(session, { ...req, force: true }, {
      title: '仍然生成',
      anchorKey,
      anchorCp: req.commandRange?.end ?? req.selection?.end ?? req.cursor?.offset ?? 0,
      block: !!req.newParagraphKind,
      paragraphKind: s.paragraphKind,
    })
  }

  // ---------- 编辑器里的显示 ----------

  function widgetsFor(articleId: number, session: ArticleSession): WidgetSpec[] {
    const out: WidgetSpec[] = []
    for (const st of streams) {
      if (st.articleId !== articleId) continue
      out.push({
        id: st.requestId,
        key: st.anchorKey,
        cp: st.anchorCp,
        phase: 'streaming',
        title: st.title,
        text: st.text,
        block: st.block,
        onCancel: () => cancel(st.requestId),
      })
    }
    const rows = session.localRows()
    for (const s of list(articleId)) {
      if (s.op === 'advice' || !s.target) continue
      let key: string | null = null
      let cp = 0
      let block = false
      if ('paragraphId' in s.target) {
        key = session.sync.keyOf(s.target.paragraphId)
        cp = s.target.end
      } else if (s.target.afterParagraphId === null) {
        key = rows[0]?.key ?? null
        cp = 0
      } else {
        key = session.sync.keyOf(s.target.afterParagraphId)
        block = true
      }
      if (!key) continue
      out.push({
        id: `s${s.id}`,
        key,
        cp,
        phase: 'pending',
        title: s.instruction || '建议',
        text: s.text,
        block,
        onReview: () => ui.review(s.id),
      })
    }
    return out
  }

  function targetsFor(articleId: number, session: ArticleSession): TargetSpec[] {
    const out: TargetSpec[] = []
    for (const s of list(articleId)) {
      if (s.op !== 'replace' || !s.target || !('paragraphId' in s.target)) continue
      const key = session.sync.keyOf(s.target.paragraphId)
      if (key) out.push({ key, start: s.target.start, end: s.target.end, focused: ui.reviewing === s.id })
    }
    return out
  }

  return {
    byArticle,
    loadState,
    streams,
    textAnalysis,
    degraded,
    busy,
    attach,
    detach,
    list,
    find,
    load,
    request,
    cancel,
    cancelLatest,
    decide,
    decideAll,
    applyWarning,
    apply,
    reject,
    force,
    refreshEditor,
    analyzeSuggestion,
  }
})
