<script setup lang="ts">
import { mdiBookOpenPageVariantOutline, mdiCogOutline, mdiFileTreeOutline, mdiPaperclip } from '@mdi/js'
import { useEditorStore } from '../stores/editor'
import { useUiStore, type Activity } from '../stores/ui'

const ui = useUiStore()
const editor = useEditorStore()

// 图标按内容选：文集树 / 回形针（参考素材）/ 打开的书（写作指引）/ 齿轮（设置）。
const items: { id: Activity; icon: string; label: string; key: string }[] = [
  { id: 'explorer', icon: mdiFileTreeOutline, label: '资源管理器', key: 'Ctrl+Shift+E' },
  { id: 'materials', icon: mdiPaperclip, label: '素材', key: '' },
  { id: 'guides', icon: mdiBookOpenPageVariantOutline, label: '写作指引', key: '' },
]
</script>

<template>
  <nav class="sk-activity bg-sk-chrome" aria-label="活动栏">
    <v-tooltip v-for="it in items" :key="it.id" location="end" :text="it.key ? `${it.label}（${it.key}）` : it.label">
      <template #activator="{ props: tp }">
        <v-btn
          v-bind="tp"
          type="button"
          class="sk-act"
          :active="ui.activity === it.id && ui.layout.sidebarOpen"
          :color="ui.activity === it.id && ui.layout.sidebarOpen ? 'primary' : undefined"
          :icon="it.icon"
          variant="text"
          :aria-label="it.label"
          :aria-pressed="ui.activity === it.id && ui.layout.sidebarOpen"
          @click="ui.showActivity(it.id)"
        />
      </template>
    </v-tooltip>
    <div class="sk-act-spacer" />
    <v-tooltip location="end" text="设置（Ctrl+,）">
      <template #activator="{ props: tp }">
        <v-btn
          v-bind="tp"
          type="button"
          class="sk-act"
          :active="editor.activeTab?.type === 'settings'"
          :icon="mdiCogOutline"
          aria-label="设置"
          @click="editor.openSettings()"
        />
      </template>
    </v-tooltip>
  </nav>
</template>

<style scoped>
.sk-activity {
  flex: none;
  width: 64px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding-block: 8px;
  border-right: 1px solid var(--sk-line);
}
.sk-act {
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: 0;
  cursor: pointer;
  color: var(--sk-muted);
}
.sk-act:hover {
  color: rgb(var(--v-theme-on-surface));
}
/* 激活态用左侧 2px 标出“当前打开的是哪个面板”，这是状态，不是装饰。 */
.sk-act-on {
  color: rgb(var(--v-theme-on-surface));
  border-left-color: rgb(var(--v-theme-primary));
}
.sk-act:focus-visible {
  outline-offset: -3px;
}
.sk-act-spacer {
  flex: 1;
}
</style>
