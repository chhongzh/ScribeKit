import { defineStore } from 'pinia'
import { computed, markRaw, reactive, ref, shallowReactive } from 'vue'
import { api, describeError } from '../api/http'
import type { CollectionDetail } from '../api/types'
import { ArticleSession } from '../editor/session'
import { useAnalysisStore } from './analysis'
import { useSettingsStore } from './settings'
import { useSuggestionStore } from './suggestions'

export type Tab =
  | { type: 'article'; id: number }
  | { type: 'outline'; id: number }
  | { type: 'guide'; id: string }
  | { type: 'settings'; id: 'settings' }

export function tabKey(t: Tab): string {
  return `${t.type}:${t.id}`
}

export type TabCloseMode = 'current' | 'right' | 'all' | 'others'

export function tabsToClose(tabs: readonly Tab[], target: Tab, mode: TabCloseMode): Tab[] {
  const index = tabs.findIndex((tab) => tabKey(tab) === tabKey(target))
  if (index < 0) return []
  switch (mode) {
    case 'current': return tabs.slice(index, index + 1)
    case 'right': return tabs.slice(index + 1)
    case 'all': return [...tabs]
    case 'others': return tabs.filter((tab) => tabKey(tab) !== tabKey(target))
  }
}

interface OpenState {
  loading: boolean
  error: string
}

/** 编辑器标签页与每篇打开文章的 session。 */
export const useEditorStore = defineStore('editor', () => {
  const tabs = ref<Tab[]>([])
  const active = ref<string | null>(null)
  const closingTabs = ref(false)
  const sessions = shallowReactive(new Map<number, ArticleSession>())
  const open = reactive<Record<string, OpenState>>({})
  const outlines = reactive<Record<number, CollectionDetail>>({})
  const guideTitles = reactive<Record<string, string>>({})

  const activeTab = computed(() => tabs.value.find((t) => tabKey(t) === active.value) ?? null)
  const activeSession = computed(() => {
    const t = activeTab.value
    return t?.type === 'article' ? (sessions.get(t.id) ?? null) : null
  })

  function activate(t: Tab) {
    if (!tabs.value.some((x) => tabKey(x) === tabKey(t))) tabs.value.push(t)
    active.value = tabKey(t)
  }

  async function openArticle(id: number) {
    const t: Tab = { type: 'article', id }
    activate(t)
    if (sessions.has(id)) return
    await loadArticle(id)
  }

  async function loadArticle(id: number) {
    const k = `article:${id}`
    open[k] = { loading: true, error: '' }
    try {
      const detail = await api.getArticle(id)
      if (!tabs.value.some((tab) => tabKey(tab) === k)) return
      const session = markRaw(new ArticleSession(detail))
      sessions.set(id, session)
      useAnalysisStore().attach(session)
      const sg = useSuggestionStore()
      sg.attach(session)
      void sg.load(id).then(() => sg.refreshEditor(id))
      open[k] = { loading: false, error: '' }
      if (useSettingsStore().settings?.autoAnalyze) {
        const analysis = useAnalysisStore()
        void analysis.analyzeParagraphs(
          session,
          session.localRows().map((r) => r.key),
        )
      }
    } catch (e) {
      open[k] = { loading: false, error: describeError(e) }
    }
  }

  async function reloadArticle(id: number) {
    const session = sessions.get(id)
    if (!session) return loadArticle(id)
    const detail = await api.getArticle(id)
    session.reload(detail)
    useSuggestionStore().refreshEditor(id)
  }

  async function openOutline(collectionId: number) {
    const t: Tab = { type: 'outline', id: collectionId }
    activate(t)
    const k = tabKey(t)
    open[k] = { loading: true, error: '' }
    try {
      outlines[collectionId] = await api.getCollection(collectionId)
      open[k] = { loading: false, error: '' }
    } catch (e) {
      open[k] = { loading: false, error: describeError(e) }
    }
  }

  function openGuide(id: string) {
    activate({ type: 'guide', id })
  }

  function openSettings() {
    activate({ type: 'settings', id: 'settings' })
  }

  /** 关闭标签页。有未保存的改动时先尝试保存，失败则返回 false 由调用方提示。 */
  async function close(t: Tab, discard = false): Promise<boolean> {
    if (t.type === 'article') {
      const s = sessions.get(t.id)
      if (s && !discard) {
        try {
          await s.sync.flush()
        } catch {
          return false
        }
      }
      if (s) {
        s.destroy()
        sessions.delete(t.id)
        useAnalysisStore().detach(t.id)
        useSuggestionStore().detach(t.id)
      }
    }
    const i = tabs.value.findIndex((x) => tabKey(x) === tabKey(t))
    if (i < 0) return true
    tabs.value.splice(i, 1)
    delete open[tabKey(t)]
    if (active.value === tabKey(t)) {
      const next = tabs.value[Math.min(i, tabs.value.length - 1)]
      active.value = next ? tabKey(next) : null
    }
    return true
  }

  async function closeTabs(target: Tab, mode: TabCloseMode, confirmDiscard: (tab: Tab) => Promise<boolean>): Promise<boolean> {
    if (closingTabs.value) return false
    const targets = tabsToClose(tabs.value, target, mode)
    closingTabs.value = true
    try {
      for (const tab of targets) {
        if (await close(tab)) continue
        if (!await confirmDiscard(tab)) return false
        await close(tab, true)
      }
      return true
    } finally {
      closingTabs.value = false
    }
  }

  /** 文章或文集在别处被删除时，关掉对应标签页。 */
  function forget(t: Tab) {
    void close(t, true)
  }

  return {
    tabs,
    active,
    closingTabs,
    sessions,
    open,
    outlines,
    guideTitles,
    activeTab,
    activeSession,
    activate,
    openArticle,
    loadArticle,
    reloadArticle,
    openOutline,
    openGuide,
    openSettings,
    close,
    closeTabs,
    forget,
  }
})
