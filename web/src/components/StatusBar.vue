<script setup lang="ts">
import { mdiAlertOutline, mdiWeatherNight, mdiWhiteBalanceSunny } from '@mdi/js'
import { computed } from 'vue'
import { ws } from '../api/ws'
import { kindLabel } from '../core/labels'
import { useEditorStore } from '../stores/editor'
import { useSettingsStore } from '../stores/settings'
import { useSuggestionStore } from '../stores/suggestions'

const editor = useEditorStore()
const settings = useSettingsStore()
const sg = useSuggestionStore()
const wsState = ws.state

const session = computed(() => editor.activeSession)
const cursor = computed(() => session.value?.ui.cursor ?? null)
const sync = computed(() => session.value?.sync.state ?? null)
const streaming = computed(() => (session.value ? sg.streams.filter((x) => x.articleId === session.value!.articleId).length : 0))

const wsText = computed(() => {
  if (wsState.status === 'open') return '已连接'
  if (wsState.status === 'connecting') return '连接中'
  return wsState.retryIn > 0 ? `已断开，${wsState.retryIn} 秒后重连` : '已断开'
})

const modelText = computed(() => {
  if (!settings.settings) return settings.error ? '读不到设置' : '读取设置中'
  return settings.modelReady ? `模型：${settings.settings.scaffolderModel}` : '未配置模型'
})
</script>

<template>
  <footer class="sk-status bg-sk-chrome" aria-label="状态栏">
    <button type="button" class="sk-si" :title="wsState.lastError || 'WebSocket /api/ws'" @click="ws.reconnectNow()">
      <span class="sk-dot" :class="`sk-dot-${wsState.status}`" aria-hidden="true" />
      {{ wsText }}<template v-if="wsState.status === 'closed'">（点击立即重连）</template>
    </button>
    <button type="button" class="sk-si"  @click="editor.openSettings()">
      <v-icon v-if="settings.settings && !settings.modelReady" :icon="mdiAlertOutline" size="14" />
      {{ modelText }}
    </button>
    <span v-if="streaming" class="sk-si">Scaffolder 生成中 · Esc 取消</span>
    <span v-if="sync" class="sk-si" role="status">
      <template v-if="sync.status === 'saving'">保存中</template>
      <span v-else-if="sync.status === 'error'" class="d-inline-flex align-center ga-1"><v-icon :icon="mdiAlertOutline" size="14" />保存失败</span>
      <template v-else>已保存</template>
    </span>

    <span class="sk-fill" />

    <template v-if="session && cursor">
      <span class="sk-si" title="当前段落类型：点击段落左侧的类型标签切换">{{ kindLabel(cursor.kind) }}</span>
      <span class="sk-si">
        第 {{ cursor.index + 1 }} 段<template v-if="cursor.sentence !== null"> · 第 {{ cursor.sentence + 1 }} 句 · 第 {{ (cursor.half ?? 0) + 1 }} 半句</template>
      </span>
      <span class="sk-si" title="正文字数：不计空白，不计 [ ] 指令">{{ session.ui.charCount }} 字</span>
    </template>
    <button
      type="button"
      class="sk-si"
      :aria-label="settings.mode === 'light' ? '切换到暗色' : '切换到亮色'"
      @click="settings.setMode(settings.mode === 'light' ? 'dark' : 'light')"
    >
      <v-icon :icon="settings.mode === 'light' ? mdiWeatherNight : mdiWhiteBalanceSunny" size="14" />
      {{ settings.mode === 'light' ? '亮色' : '暗色' }}
    </button>
  </footer>
</template>

<style scoped>
.sk-status {
  flex: none;
  display: flex;
  align-items: center;
  height: 24px;
  font-size: 12px;
  border-top: 1px solid var(--sk-line);
  overflow: hidden;
  white-space: nowrap;
}
.sk-si {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 24px;
  padding: 0 8px;
  background: none;
  border: 0;
  color: inherit;
  font: inherit;
}
button.sk-si {
  cursor: pointer;
}
button.sk-si:hover {
  background-color: rgba(var(--v-theme-on-surface), var(--v-hover-opacity));
}
button.sk-si:focus-visible {
  outline-offset: -2px;
}
.sk-fill {
  flex: 1;
}
/* 连接状态点：标示真实的连接状态，并配有文字。 */
.sk-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  border: 1px solid currentColor;
}
.sk-dot-open {
  background-color: currentColor;
}
.sk-dot-connecting {
  background-color: rgba(var(--v-theme-on-surface), 0.3);
}
</style>
