<script setup lang="ts">
import { mdiClose } from '@mdi/js'
import { computed } from 'vue'
import { useDialogStore } from '../../stores/dialogs'
import { tabKey, useEditorStore, type Tab } from '../../stores/editor'
import { useWorkspaceStore } from '../../stores/workspace'
import SettingsView from '../settings/SettingsView.vue'
import StateBlock from '../StateBlock.vue'
import ArticleEditor from './ArticleEditor.vue'
import GuideView from './GuideView.vue'
import OutlineEditor from './OutlineEditor.vue'

const editor = useEditorStore()
const workspace = useWorkspaceStore()
const dialogs = useDialogStore()

function label(t: Tab): string {
  switch (t.type) {
    case 'article': {
      const s = editor.sessions.get(t.id)
      return (s?.meta.title ?? workspace.findSummary(t.id)?.title) || '未命名'
    }
    case 'outline':
      return `总纲 · ${editor.outlines[t.id]?.name ?? workspace.collections.find((c) => c.id === t.id)?.name ?? ''}`
    case 'guide':
      return `指引 · ${editor.guideTitles[t.id] ?? t.id}`
    case 'settings':
      return '设置'
  }
}

function dirty(t: Tab): boolean {
  if (t.type !== 'article') return false
  const s = editor.sessions.get(t.id)
  return !!s && (s.sync.state.status === 'saving' || s.sync.state.status === 'error')
}

async function close(t: Tab) {
  const ok = await editor.close(t)
  if (ok) return
  const discard = await dialogs.confirm({
    title: '有改动没有保存',
    text: '这篇文章有改动没能写入后端。直接关闭会丢掉这些改动。',
    confirm: '丢弃改动并关闭',
    danger: true,
  })
  if (discard) await editor.close(t, true)
}

function onTabKey(e: KeyboardEvent, i: number) {
  const n = editor.tabs.length
  let j = -1
  if (e.key === 'ArrowRight') j = (i + 1) % n
  else if (e.key === 'ArrowLeft') j = (i - 1 + n) % n
  else if (e.key === 'Delete') {
    e.preventDefault()
    void close(editor.tabs[i]!)
    return
  } else return
  e.preventDefault()
  const t = editor.tabs[j]!
  editor.activate(t)
  ;(document.getElementById(`sk-tab-${tabKey(t)}`) as HTMLElement | null)?.focus()
}

const hasTabs = computed(() => editor.tabs.length > 0)
</script>

<template>
  <section class="sk-editor-area bg-sk-editor" aria-label="编辑器">
    <div v-if="hasTabs" class="sk-tabs bg-sk-panel" role="tablist" aria-label="打开的标签页">
      <div
        v-for="(t, i) in editor.tabs"
        :key="tabKey(t)"
        class="sk-tab"
        :class="{ 'sk-tab-on': editor.active === tabKey(t) }"
        @auxclick.middle="close(t)"
      >
        <button
          :id="`sk-tab-${tabKey(t)}`"
          type="button"
          role="tab"
          class="sk-tab-main"
          :aria-selected="editor.active === tabKey(t)"
          :tabindex="editor.active === tabKey(t) ? 0 : -1"
          @click="editor.activate(t)"
          @keydown="onTabKey($event, i)"
        >
          <span class="sk-tab-label">{{ label(t) }}</span>
          <span v-if="dirty(t)" class="sk-tab-dirty" aria-label="有未保存的改动">●</span>
        </button>
        <button type="button" class="sk-tab-close" :aria-label="`关闭 ${label(t)}`" title="关闭（Delete）" @click="close(t)">
          <v-icon :icon="mdiClose" size="14" />
        </button>
      </div>
    </div>

    <div class="sk-tab-body">
      <div v-if="!hasTabs" class="sk-welcome">
        <h1 class="sk-welcome-title">ScribeKit</h1>
        <p class="sk-muted">在左侧资源管理器里打开一篇文章，或者新建一篇。</p>
        <ul class="sk-welcome-keys sk-muted">
          <li><span class="sk-kbd">Enter</span> 放下这一段，开始下一段</li>
          <li><span class="sk-kbd">Shift+Enter</span> 把光标处交给 Scaffolder</li>
          <li><span class="sk-kbd">Ctrl+K</span> 在光标处打开命令面板</li>
          <li><span class="sk-kbd">Ctrl+B</span> 侧边栏 · <span class="sk-kbd">Ctrl+J</span> 底部面板 · <span class="sk-kbd">Ctrl+Alt+B</span> 检查器</li>
        </ul>
      </div>

      <template v-for="t in editor.tabs" :key="tabKey(t)">
        <div v-show="editor.active === tabKey(t)" class="sk-tab-pane" role="tabpanel" :aria-label="label(t)">
          <template v-if="t.type === 'article'">
            <StateBlock v-if="editor.open[tabKey(t)]?.loading" state="loading" text="正在打开文章" />
            <StateBlock v-else-if="editor.open[tabKey(t)]?.error" state="error" :text="`文章打不开：${editor.open[tabKey(t)]?.error}`">
              <v-btn variant="outlined" size="small" @click="editor.loadArticle(t.id)">再试一次</v-btn>
              <v-btn size="small" @click="close(t)">关闭标签页</v-btn>
            </StateBlock>
            <ArticleEditor v-else-if="editor.sessions.get(t.id)" :session="editor.sessions.get(t.id)!" :active="editor.active === tabKey(t)" />
          </template>
          <OutlineEditor v-else-if="t.type === 'outline'" :collection-id="t.id" />
          <GuideView v-else-if="t.type === 'guide'" :guide-id="t.id" />
          <SettingsView v-else-if="t.type === 'settings'" />
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.sk-editor-area {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.sk-tabs {
  flex: none;
  display: flex;
  overflow-x: auto;
  border-bottom: 1px solid var(--sk-line);
  scrollbar-width: thin;
}
.sk-tab {
  flex: none;
  display: flex;
  align-items: center;
  max-width: 240px;
  border-right: 1px solid var(--sk-line);
  font-size: 13px;
  color: var(--sk-muted);
}
.sk-tab-on {
  color: rgb(var(--v-theme-on-surface));
  background-color: rgb(var(--v-theme-sk-editor));
  box-shadow: inset 0 2px 0 rgb(var(--v-theme-primary));
}
.sk-tab-main {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  height: 35px;
  padding: 0 4px 0 12px;
  background: none;
  border: 0;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.sk-tab-main:focus-visible,
.sk-tab-close:focus-visible {
  outline-offset: -2px;
}
.sk-tab-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sk-tab-dirty {
  font-size: 10px;
}
.sk-tab-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  margin-right: 4px;
  background: none;
  border: 0;
  color: inherit;
  border-radius: 3px;
  cursor: pointer;
}
.sk-tab-close:hover {
  background-color: rgba(var(--v-theme-on-surface), var(--v-hover-opacity));
}
.sk-tab-body {
  flex: 1;
  min-height: 0;
  position: relative;
}
.sk-tab-pane {
  position: absolute;
  inset: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.sk-welcome {
  max-width: 520px;
  margin: 12vh auto 0;
  padding: 0 24px;
  font-size: 14px;
}
.sk-welcome-title {
  font-family: var(--sk-font-body);
  font-weight: 600;
  font-size: 28px;
  margin: 0 0 8px;
}
.sk-welcome-keys {
  list-style: none;
  padding: 0;
  margin: 24px 0 0;
  line-height: 2.2;
}
</style>
