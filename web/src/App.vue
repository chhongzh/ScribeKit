<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import { ws } from './api/ws'
import ActivityBar from './components/ActivityBar.vue'
import DialogHost from './components/DialogHost.vue'
import NoticeHost from './components/NoticeHost.vue'
import SplitHandle from './components/SplitHandle.vue'
import StatusBar from './components/StatusBar.vue'
import EditorArea from './components/editor/EditorArea.vue'
import InspectorPanel from './components/inspector/InspectorPanel.vue'
import BottomPanel from './components/panel/BottomPanel.vue'
import ExplorerPanel from './components/sidebar/ExplorerPanel.vue'
import GuidesPanel from './components/sidebar/GuidesPanel.vue'
import MaterialsPanel from './components/sidebar/MaterialsPanel.vue'
import { useAnalysisStore } from './stores/analysis'
import { useEditorStore } from './stores/editor'
import { useLogStore } from './stores/log'
import { useSettingsStore } from './stores/settings'
import { useSuggestionStore } from './stores/suggestions'
import { useUiStore } from './stores/ui'
import { useWorkspaceStore } from './stores/workspace'

const ui = useUiStore()
const settings = useSettingsStore()
const workspace = useWorkspaceStore()
const editor = useEditorStore()
const analysis = useAnalysisStore()
const suggestions = useSuggestionStore()
const log = useLogStore()

ws.log = (dir, text, msg) => log.add(dir, text, msg)

function onKey(e: KeyboardEvent) {
  const mod = e.ctrlKey || e.metaKey
  // 焦点不在正文里时 Esc 也能取消正在生成的建议；编辑器、对话框自己处理过的 Esc 不再重复处理。
  if (e.key === 'Escape' && !e.defaultPrevented) {
    const s = editor.activeSession
    if (s && suggestions.cancelLatest(s.articleId)) e.preventDefault()
    return
  }
  if (mod && e.shiftKey && e.code === 'KeyE') {
    e.preventDefault()
    ui.showActivity('explorer')
  } else if (mod && e.code === 'KeyB' && !e.shiftKey) {
    e.preventDefault()
    ui.layout.sidebarOpen = !ui.layout.sidebarOpen
  } else if (mod && e.code === 'KeyJ') {
    e.preventDefault()
    ui.layout.panelOpen = !ui.layout.panelOpen
  } else if (mod && e.altKey && e.code === 'KeyB') {
    e.preventDefault()
    ui.layout.inspectorOpen = !ui.layout.inspectorOpen
  } else if (mod && e.code === 'Comma') {
    e.preventDefault()
    editor.openSettings()
  }
}

// 窄屏（平板竖屏、手机）上默认收起侧边栏和检查器，把宽度留给稿面。
const narrow = window.matchMedia('(max-width: 900px)')
function fitNarrow() {
  if (narrow.matches) {
    ui.layout.sidebarOpen = false
    ui.layout.inspectorOpen = false
  }
}

onMounted(async () => {
  fitNarrow()
  narrow.addEventListener('change', fitNarrow)
  window.addEventListener('keydown', onKey)
  ws.start()
  ws.on('$open', () => analysis.resume())
  await Promise.all([settings.load(), workspace.refresh()])
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  narrow.removeEventListener('change', fitNarrow)
  ws.stop()
})

const SIDEBAR_MIN = 200
const SIDEBAR_MAX = 520
</script>

<template>
  <v-app>
    <div class="sk-shell">
      <div class="sk-main">
        <ActivityBar />
        <template v-if="ui.layout.sidebarOpen">
          <aside class="sk-sidebar bg-sk-panel" :style="{ width: `${ui.layout.sidebarWidth}px` }" aria-label="侧边栏">
            <ExplorerPanel v-if="ui.activity === 'explorer'" />
            <MaterialsPanel v-else-if="ui.activity === 'materials'" />
            <GuidesPanel v-else-if="ui.activity === 'guides'" />
          </aside>
          <SplitHandle
            v-model="ui.layout.sidebarWidth"
            direction="vertical"
            :min="SIDEBAR_MIN"
            :max="SIDEBAR_MAX"
            label="调整侧边栏宽度"
          />
        </template>
        <div class="sk-center">
          <EditorArea class="sk-editor-area" />
          <template v-if="ui.layout.panelOpen">
            <SplitHandle
              v-model="ui.layout.panelHeight"
              direction="horizontal"
              :min="96"
              :max="520"
              invert
              label="调整底部面板高度"
            />
            <BottomPanel class="sk-bottom" :style="{ height: `${ui.layout.panelHeight}px` }" />
          </template>
        </div>
        <template v-if="ui.layout.inspectorOpen">
          <SplitHandle
            v-model="ui.layout.inspectorWidth"
            direction="vertical"
            :min="260"
            :max="560"
            invert
            label="调整检查器宽度"
          />
          <InspectorPanel class="sk-inspector" :style="{ width: `${ui.layout.inspectorWidth}px` }" />
        </template>
      </div>
      <StatusBar />
    </div>
    <DialogHost />
    <NoticeHost />
  </v-app>
</template>

<style scoped>
.sk-shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
}
.sk-main {
  flex: 1;
  display: flex;
  min-height: 0;
}
.sk-sidebar {
  flex: none;
  min-width: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--sk-line);
  overflow: hidden;
}
.sk-center {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.sk-editor-area {
  flex: 1;
  min-height: 0;
}
.sk-bottom {
  flex: none;
}
.sk-inspector {
  flex: none;
  border-left: 1px solid var(--sk-line);
}
</style>
