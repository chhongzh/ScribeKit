<script setup lang="ts">
import { mdiAlertOutline, mdiWeatherNight, mdiWhiteBalanceSunny } from '@mdi/js'
import { computed } from 'vue'
import { ws } from '../api/ws'
import { kindLabel } from '../core/labels'
import { useEditorStore } from '../stores/editor'
import { useAnalysisStore } from '../stores/analysis'
import { useSettingsStore } from '../stores/settings'
import { useSuggestionStore } from '../stores/suggestions'
import { useUiStore } from '../stores/ui'
import { useNotifyStore } from '../stores/notify'

const editor = useEditorStore()
const settings = useSettingsStore()
const sg = useSuggestionStore()
const ui = useUiStore()
const analysis = useAnalysisStore()
const notify = useNotifyStore()
const wsState = ws.state

const session = computed(() => editor.activeSession)
const cursor = computed(() => session.value?.ui.cursor ?? null)
const sync = computed(() => session.value?.sync.state ?? null)
const metadataSave = computed(() => session.value?.metadataSave ?? null)
const streaming = computed(() => (session.value ? sg.streams.filter((x) => x.articleId === session.value!.articleId).length : 0))
const analysisState = computed(() => (session.value ? analysis.st(session.value.articleId) : null))
const analyzing = computed(() => !!(analysisState.value?.inflight || analysisState.value?.preparing || analysisState.value?.articleLoading))
const usageTitle = computed(() => `本次服务启动以来后台 Agent 的实际用量：输入 ${wsState.promptTokens}，输出 ${wsState.completionTokens}。流式用量每 100 毫秒合并更新；只在响应末尾返回用量的服务商会在该轮结束时更新。${wsState.missingUsage ? `有 ${wsState.missingUsage} 次响应未返回用量，未计入。` : ''}`)

async function reloadArticle() {
  if (!session.value) return
  try {
    await editor.reloadArticle(session.value.articleId)
  } catch (error) {
    notify.error(`重新载入失败：${error instanceof Error ? error.message : String(error)}`)
  }
}

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
    <v-btn size="small" :title="wsState.lastError || 'WebSocket /api/ws'" @click="ws.reconnectNow()">
      <span class="sk-dot" :class="`sk-dot-${wsState.status}`" aria-hidden="true" />
      {{ wsText }}<template v-if="wsState.status === 'closed'">（点击立即重连）</template>
    </v-btn>
    <v-btn size="small" @click="editor.openSettings()">
      <v-icon v-if="settings.settings && !settings.modelReady" :icon="mdiAlertOutline" size="14" />
      {{ modelText }}
    </v-btn>
    <span v-if="streaming" class="sk-si">Scaffolder 生成中 · Esc 取消</span>
    <v-btn v-if="analyzing" size="small" title="点击取消当前文章的分析" @click="session && analysis.cancel(session.articleId)">Analyzer 分析中 · 取消</v-btn>
    <span v-if="analysisState?.error" class="sk-si text-error" role="alert">{{ analysisState.error }}</span>
    <v-btn size="small" :title="`${usageTitle} 点击打开后台任务。`" @click="ui.openPanel('tasks')">后台 Agent：{{ wsState.active }} 个 · 任务 {{ ws.tasks.active.length }} 个</v-btn>
    <span class="sk-si" :title="usageTitle">{{ wsState.missingUsage ? '已知用量' : '已用' }} {{ wsState.usedTokens }} tokens<template v-if="wsState.missingUsage">（部分用量未返回）</template></span>
    <v-btn size="small" @click="ui.openPanel('problems')">问题</v-btn>
    <v-btn size="small" @click="ui.openPanel('output')">输出</v-btn>
    <v-menu v-if="sync?.status === 'error' || metadataSave?.status === 'error'" location="top">
      <template #activator="{ props: menuProps }"><v-btn v-bind="menuProps" size="small" color="error" :prepend-icon="mdiAlertOutline">保存失败 · 处理</v-btn></template>
      <v-card max-width="460" title="保存失败">
        <v-card-text>
          <template v-if="sync?.status === 'error'"><p>没能{{ sync.failedOp }}：{{ sync.error }}。之后的改动暂留本地。</p><v-btn @click="session?.sync.retry()">重试正文保存</v-btn><v-btn @click="reloadArticle">放弃本地改动，重新载入</v-btn></template>
          <template v-if="metadataSave?.status === 'error'"><p>标题或梗概没有保存：{{ metadataSave.error }}</p><v-btn @click="session?.retryMetadata?.()">重试标题与梗概保存</v-btn></template>
        </v-card-text>
      </v-card>
    </v-menu>
    <span v-else-if="sync" class="sk-si" role="status">{{ sync.status === 'saving' || metadataSave?.status === 'saving' ? '保存中' : '已保存' }}</span>

    <span class="sk-fill" />

    <v-btn v-if="!ui.layout.inspectorOpen" size="small" title="展开检查器（Ctrl+Alt+B）" @click="ui.openInspector(ui.inspectorTab)">检查器</v-btn>
    <template v-if="session && cursor">
      <span class="sk-si" title="当前段落类型：点击段落左侧的类型标签切换">{{ kindLabel(cursor.kind) }}</span>
      <span class="sk-si">
        第 {{ cursor.index + 1 }} 段<template v-if="cursor.sentence !== null"> · 第 {{ cursor.sentence + 1 }} 句 · 第 {{ (cursor.half ?? 0) + 1 }} 半句</template>
      </span>
      <span class="sk-si" title="正文字数：不计空白，不计 [ ] 指令">{{ session.ui.charCount }} 字</span>
    </template>
    <v-menu v-if="session" location="top end">
      <template #activator="{ props: menuProps }">
        <v-btn v-bind="menuProps" size="small" aria-label="正文缩放">{{ ui.editorZoom }}%</v-btn>
      </template>
      <v-list aria-label="正文缩放比例" density="compact">
        <v-list-item v-for="zoom in [75, 90, 100, 110, 125, 150]" :key="zoom" :active="ui.editorZoom === zoom" :title="zoom === 100 ? '100%（默认）' : `${zoom}%`" @click="ui.editorZoom = zoom" />
      </v-list>
    </v-menu>
    <v-btn
      type="button"
      size="small"
      :aria-label="settings.mode === 'light' ? '切换到暗色' : '切换到亮色'"
      @click="settings.setMode(settings.mode === 'light' ? 'dark' : 'light')"
    >
      <v-icon :icon="settings.mode === 'light' ? mdiWeatherNight : mdiWhiteBalanceSunny" size="14" />
      {{ settings.mode === 'light' ? '亮色' : '暗色' }}
    </v-btn>
  </footer>
</template>

<style scoped>
.sk-status {
  flex: none;
  display: flex;
  align-items: center;
  min-height: 40px;
  font-size: 12px;
  border-top: 1px solid var(--sk-line);
  flex-wrap: wrap;
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
