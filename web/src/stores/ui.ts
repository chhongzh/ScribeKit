import { defineStore } from 'pinia'
import { reactive, ref, watch } from 'vue'

export type Activity = 'explorer' | 'materials' | 'guides'
export type PanelTab = 'problems' | 'output'
export type InspectorTab = 'suggestions' | 'distribution' | 'structure'

const KEY = 'scribekit.layout'

interface Layout {
  sidebarWidth: number
  inspectorWidth: number
  panelHeight: number
  sidebarOpen: boolean
  inspectorOpen: boolean
  panelOpen: boolean
  editorWidth: number
}

function load(): Layout {
  const d: Layout = {
    sidebarWidth: 280,
    inspectorWidth: 340,
    panelHeight: 200,
    sidebarOpen: true,
    inspectorOpen: true,
    panelOpen: true,
    editorWidth: 880,
  }
  try {
    const raw = localStorage.getItem(KEY)
    if (raw) {
      const saved = JSON.parse(raw) as Partial<Layout>
      const layout = { ...d, ...saved }
      layout.inspectorOpen = d.inspectorOpen
      layout.editorWidth = typeof saved.editorWidth === 'number' && Number.isFinite(saved.editorWidth) ? Math.max(560, Math.min(1600, saved.editorWidth)) : d.editorWidth
      return layout
    }
  } catch {
    // 布局偏好损坏时用默认值。
  }
  return d
}

export const useUiStore = defineStore('ui', () => {
  const layout = reactive(load())
  const activity = ref<Activity>('explorer')
  const panelTab = ref<PanelTab>('problems')
  const inspectorTab = ref<InspectorTab>('suggestions')
  /** 当前在检查器里评审的建议 id。 */
  const reviewing = ref<number | null>(null)
  const editorZoom = ref(100)

  watch(layout, (v) => localStorage.setItem(KEY, JSON.stringify(v)), { deep: true })

  function showActivity(a: Activity) {
    if (activity.value === a && layout.sidebarOpen) {
      layout.sidebarOpen = false
      return
    }
    activity.value = a
    layout.sidebarOpen = true
  }

  function openPanel(tab: PanelTab) {
    panelTab.value = tab
    layout.panelOpen = true
  }

  function openInspector(tab: InspectorTab) {
    inspectorTab.value = tab
    layout.inspectorOpen = true
  }

  function review(id: number) {
    reviewing.value = id
    openInspector('suggestions')
  }

  return { layout, activity, panelTab, inspectorTab, reviewing, editorZoom, showActivity, openPanel, openInspector, review }
})
