<script setup lang="ts">
import { mdiConsoleLine, mdiExportVariant, mdiPaletteSwatchOutline } from '@mdi/js'
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import type { ParagraphKind } from '../../api/types'
import { DESC_KINDS, PARAGRAPH_KINDS, kindLabel } from '../../core/labels'
import { exportArticle } from '../../editor/exporter'
import { blockedReason, degradeNote, planShiftEnter, prepare, type ActionInput } from '../../editor/scaffold'
import type { ArticleSession } from '../../editor/session'
import { useAnalysisStore } from '../../stores/analysis'
import { useEditorStore } from '../../stores/editor'
import { useNotifyStore } from '../../stores/notify'
import { useSettingsStore } from '../../stores/settings'
import { useSuggestionStore } from '../../stores/suggestions'
import { useUiStore } from '../../stores/ui'
import ArticleHeader from './ArticleHeader.vue'
import CommandPalette from './CommandPalette.vue'
import HoverCard from './HoverCard.vue'
import RelationOverlay from './RelationOverlay.vue'

const props = defineProps<{ session: ArticleSession; active: boolean }>()

const analysis = useAnalysisStore()
const sg = useSuggestionStore()
const ui = useUiStore()
const editor = useEditorStore()
const notify = useNotifyStore()
const settings = useSettingsStore()

const host = ref<HTMLElement | null>(null)
const paper = ref<HTMLElement | null>(null)
const s = props.session
const layers = s.ui.layers

// ---------- 段落类型菜单 ----------
const kindMenu = ref<{ key: string; target: HTMLElement | [number, number]; mode: 'set' | 'new' } | null>(null)
const kindMenuOpen = computed({
  get: () => !!kindMenu.value,
  set: (v) => {
    if (!v) {
      kindMenu.value = null
      s.focus()
    }
  },
})
const kindMenuNode = computed(() => (kindMenu.value ? s.find(kindMenu.value.key) : null))

// 菜单打开后把焦点移进去，键盘可以直接上下选（Alt+T、Alt+Enter 打开时尤其需要）。
watch(kindMenu, async (v) => {
  if (!v) return
  await nextTick()
  setTimeout(() => {
    const item = document.querySelector<HTMLElement>('.sk-kind-menu .v-list-item--active, .sk-kind-menu .v-list-item')
    item?.focus()
  }, 30)
})

function setKind(kind: ParagraphKind) {
  if (kindMenu.value) s.setKind(kindMenu.value.key, kind)
  kindMenuOpen.value = false
}
function newAfter(kind: ParagraphKind) {
  const key = kindMenu.value?.key
  kindMenu.value = null
  if (key) s.insertAfter(key, kind)
}
function moveFromMenu(delta: number) {
  const key = kindMenu.value?.key
  kindMenuOpen.value = false
  if (key) s.moveKey(key, delta)
}
function deleteFromMenu() {
  const key = kindMenu.value?.key
  kindMenuOpen.value = false
  if (key) s.deleteParagraph(key)
}

// ---------- 命令面板 ----------
const palette = ref<{ at: { left: number; top: number }; reason: string } | null>(null)

function openPalette(reason = '光标即命令面板：写一句指令，或选一个动作。') {
  const c = s.caretCoords()
  const rect = host.value?.getBoundingClientRect()
  palette.value = { at: c ? { left: c.left, top: c.bottom } : { left: (rect?.left ?? 0) + 40, top: (rect?.top ?? 0) + 40 }, reason }
}
function closePalette() {
  palette.value = null
  s.focus()
}

async function runAction(a: ActionInput) {
  palette.value = null
  const c = s.ui.cursor
  if (!c) return
  const blocked = blockedReason(s)
  if (blocked) {
    notify.push(`${blocked}。`)
    s.focus()
    return
  }
  const note = degradeNote(c, a)
  if (note) notify.push(note, { timeout: 7000 })
  const p = prepare(s, c, a)
  s.focus()
  await sg.request(s, p.draft, p.view)
}

function onShiftEnter() {
  const c = s.ui.cursor
  if (!c) return
  const blocked = blockedReason(s)
  const plan = planShiftEnter(c)
  if (plan.kind === 'palette') {
    openPalette(plan.reason)
    return
  }
  if (blocked) {
    notify.push(`${blocked}。`)
    return
  }
  void runAction(plan.action)
}

// ---------- 挂载 ----------
onMounted(() => {
  if (!host.value) return
  s.mount(host.value, {
    onKindMenu: (key, anchor) => (kindMenu.value = { key, target: anchor, mode: 'set' }),
    onNewParagraphMenu: (key) => {
      const c = s.caretCoords()
      kindMenu.value = { key, target: c ? [c.left, c.bottom] : [200, 200], mode: 'new' }
    },
    onPalette: () => openPalette(),
    onShiftEnter,
    onEscape: () => {
      if (palette.value) {
        closePalette()
        return true
      }
      return sg.cancelLatest(s.articleId)
    },
    onTextEdited: (keys) => analysis.schedule(s, keys),
  })
  sg.refreshEditor(s.articleId)
})

watch(
  () => props.active,
  async (v) => {
    if (v) {
      await nextTick()
      s.refresh()
    }
  },
)
watch(layers, () => s.refresh(), { deep: true })
watch(
  () => [sg.list(s.articleId).length, sg.streams.length, ui.reviewing],
  () => sg.refreshEditor(s.articleId),
)

// ---------- 工具条 ----------
const st = computed(() => analysis.st(s.articleId))
const analyzing = computed(() => st.value.inflight > 0 || st.value.articleLoading)
const syncState = s.sync.state
const stale = computed(() => {
  void s.ui.revision
  const c = s.ui.cursor
  return c ? s.hasStaleAnalysis(c.key) : false
})

function analyzeCurrent() {
  const c = s.ui.cursor
  analysis.resume()
  if (c) void analysis.analyzeParagraphs(s, [c.key])
}
function analyzeAll() {
  analysis.resume()
  void analysis.analyzeArticle(s)
}

async function doExport(format: 'md' | 'docx') {
  try {
    await exportArticle(s.articleId, format, s)
  } catch {
    notify.error('有改动没有保存成功，导出会缺内容。先处理上方的保存失败。')
  }
}

async function reloadFromServer() {
  try {
    await editor.reloadArticle(s.articleId)
    notify.push('已按服务器上的版本重新载入。')
  } catch (e) {
    notify.error(`重新载入失败：${e instanceof Error ? e.message : String(e)}`)
  }
}

const layerButtons = [
  { key: 'desc', label: '描写', hint: '半句描写类型底色' },
  { key: 'marks', label: '标记', hint: '点题方框、关键词、重复、代词、呼应、论点论据论证' },
  { key: 'logic', label: '逻辑', hint: '显示全部半句关系；关闭时只显示悬停半句的关系' },
  { key: 'diag', label: '诊断', hint: '点状下划线，与底部“问题”面板对应' },
] as const

const legend = DESC_KINDS.filter((d) => d.colored)
</script>

<template>
  <div class="sk-article">
    <div class="sk-toolbar bg-sk-panel" role="toolbar" aria-label="编辑器工具条">
      <span class="sk-tb-group" role="group" aria-label="分析图层">
        <v-tooltip v-for="b in layerButtons" :key="b.key" :text="b.hint">
          <template #activator="{ props: tp }">
            <button
              v-bind="tp"
              type="button"
              class="sk-layer"
              :class="{ 'sk-layer-on': layers[b.key] }"
              :aria-pressed="layers[b.key]"
              @click="layers[b.key] = !layers[b.key]"
            >
              {{ b.label }}
            </button>
          </template>
        </v-tooltip>
        <v-menu location="bottom start">
          <template #activator="{ props: mp }">
            <v-btn v-bind="mp" :icon="mdiPaletteSwatchOutline" size="small" density="comfortable" aria-label="描写颜色图例" title="图例" />
          </template>
          <v-card class="pa-3 sk-legend" min-width="220">
            <p class="text-caption sk-muted mb-2">描写类型底色</p>
            <div v-for="d in legend" :key="d.value" class="sk-legend-row">
              <span class="sk-swatch" :class="`sk-desc-${d.value}`">样字</span>{{ d.title }}
            </div>
            <p class="text-caption sk-muted mt-3 mb-2">标记</p>
            <div class="sk-legend-row"><span class="sk-swatch sk-mk-theme">点题</span>方框：点题字眼</div>
            <div class="sk-legend-row"><span class="sk-swatch sk-mk-keyword">关键</span>实线：关键词</div>
            <div class="sk-legend-row"><span class="sk-swatch sk-mk-repeat">重复</span>波浪线：重复用词</div>
            <div class="sk-legend-row"><span class="sk-swatch sk-mk-pronoun">他</span>虚线：代词，悬停看指代</div>
            <div class="sk-legend-row"><span class="sk-swatch sk-mk-callback">呼应</span>双线：前后呼应</div>
            <div class="sk-legend-row"><span class="sk-swatch sk-diag sk-diag-warning">诊断</span>点线：诊断</div>
            <div class="sk-legend-row"><span class="sk-swatch sk-cmd">[指令]</span>给 Scaffolder 的指令</div>
          </v-card>
        </v-menu>
      </span>

      <span class="sk-tb-sep" />

      <span class="sk-tb-group">
        <v-btn size="small" :disabled="analyzing" @click="analyzeCurrent">分析本段</v-btn>
        <v-btn size="small" :disabled="analyzing" @click="analyzeAll">分析全文</v-btn>
        <span class="sk-tb-status" role="status">
          <template v-if="analyzing">Analyzer 分析中</template>
          <template v-else-if="st.errorCode === 'model_not_configured'">
            <span class="text-error">还没配置模型，分析暂停。</span>
            <button type="button" class="sk-plain text-decoration-underline" @click="editor.openSettings()">打开设置</button>
          </template>
          <span v-else-if="st.error" class="text-error">{{ st.error }}</span>
          <template v-else-if="stale">本段改动后旧分析已失效{{ settings.settings?.autoAnalyze ? '，停笔 1.2 秒后重新分析' : '' }}</template>
        </span>
      </span>

      <span class="sk-tb-fill" />

      <v-btn size="small" :prepend-icon="mdiConsoleLine" @click="openPalette()">命令面板 <span class="sk-kbd ml-1">Ctrl+K</span></v-btn>
      <v-menu location="bottom end">
        <template #activator="{ props: mp }">
          <v-btn v-bind="mp" size="small" :prepend-icon="mdiExportVariant">导出</v-btn>
        </template>
        <v-list density="compact">
          <v-list-item title="导出 Markdown（.md）" @click="doExport('md')" />
          <v-list-item title="导出 Word（.docx）" @click="doExport('docx')" />
        </v-list>
      </v-menu>
    </div>

    <div v-if="syncState.status === 'error'" class="sk-sync-error" role="alert">
      <span>没能{{ syncState.failedOp }}：{{ syncState.error }}。之后的改动先留在本地，没有写入后端。</span>
      <v-btn size="small" variant="outlined" @click="s.sync.retry()">重试</v-btn>
      <v-btn size="small" @click="reloadFromServer">放弃本地改动，按服务器版本重新载入</v-btn>
    </div>

    <div class="sk-scroll">
      <div ref="paper" class="sk-paper" :class="{ 'sk-genre-poetry': s.meta.genre === 'poetry' }">
        <ArticleHeader :session="s" @enter-body="s.focus()" />
        <div ref="host" class="sk-host" />
        <RelationOverlay :session="s" :paper="paper" />
      </div>
    </div>

    <HoverCard v-if="active" :session="s" />

    <v-menu
      v-model="kindMenuOpen"
      :target="kindMenu?.target"
      location="bottom start"
      :close-on-content-click="true"
      content-class="sk-kind-menu"
    >
      <v-card min-width="240" class="py-1">
        <template v-if="kindMenu?.mode === 'set'">
          <p class="text-caption sk-muted px-4 pt-1 mb-0">本段类型</p>
          <v-list density="compact" class="py-0">
            <v-list-item
              v-for="k in PARAGRAPH_KINDS"
              :key="k.value"
              :title="k.title"
              :subtitle="k.hint"
              :active="kindMenuNode?.node.attrs.kind === k.value"
              @click="setKind(k.value)"
            />
          </v-list>
          <v-divider class="my-1" />
          <p class="text-caption sk-muted px-4 mb-0">在下方新建</p>
          <div class="d-flex flex-wrap ga-1 px-3 pb-1">
            <v-btn v-for="k in PARAGRAPH_KINDS" :key="k.value" size="x-small" variant="outlined" @click="newAfter(k.value)">{{ k.title }}</v-btn>
          </div>
          <v-divider class="my-1" />
          <v-list density="compact" class="py-0">
            <v-list-item title="上移一段" subtitle="Alt+Shift+↑" @click="moveFromMenu(-1)" />
            <v-list-item title="下移一段" subtitle="Alt+Shift+↓" @click="moveFromMenu(1)" />
            <v-list-item title="删除本段" base-color="error" @click="deleteFromMenu" />
          </v-list>
        </template>
        <template v-else>
          <p class="text-caption sk-muted px-4 pt-1 mb-0">在下方新建哪类段落</p>
          <v-list density="compact" class="py-0">
            <v-list-item v-for="k in PARAGRAPH_KINDS" :key="k.value" :title="k.title" :subtitle="k.hint" @click="newAfter(k.value)" />
          </v-list>
        </template>
      </v-card>
    </v-menu>

    <CommandPalette
      v-if="palette && s.ui.cursor"
      :session="s"
      :cursor="s.ui.cursor"
      :at="palette.at"
      :reason="palette.reason"
      @run="runAction"
      @set-kind="(k) => { s.setKind(s.ui.cursor!.key, k); closePalette(); notify.push(`本段已设为${kindLabel(k)}。`) }"
      @close="closePalette"
    />
  </div>
</template>

<style scoped>
.sk-article {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.sk-toolbar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 4px;
  min-height: 36px;
  padding: 0 8px;
  border-bottom: 1px solid var(--sk-line);
  font-size: 13px;
  overflow-x: auto;
}
.sk-tb-group {
  display: flex;
  align-items: center;
  gap: 2px;
}
.sk-tb-sep {
  width: 1px;
  height: 18px;
  background-color: rgba(var(--v-border-color), var(--v-border-opacity));
  margin: 0 6px;
}
.sk-tb-fill {
  flex: 1;
}
.sk-tb-status {
  margin-left: 8px;
  font-size: 12px;
  color: var(--sk-muted);
  white-space: nowrap;
}
.sk-layer {
  height: 26px;
  padding: 0 10px;
  font: inherit;
  font-size: 12px;
  background: none;
  border: 1px solid var(--sk-line);
  border-radius: 3px;
  color: var(--sk-muted);
  cursor: pointer;
}
.sk-layer-on {
  color: rgb(var(--v-theme-on-surface));
  border-color: rgb(var(--v-theme-primary));
  background-color: rgba(var(--v-theme-primary), 0.1);
}
.sk-legend-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  line-height: 2;
}
.sk-swatch {
  font-family: var(--sk-font-body);
  min-width: 3.2em;
  text-align: center;
}
.sk-sync-error {
  flex: none;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 6px 12px;
  font-size: 13px;
  border-bottom: 1px solid rgb(var(--v-theme-error));
  background-color: rgba(var(--v-theme-error), 0.08);
}
.sk-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.sk-paper {
  position: relative;
  max-width: 44em;
  margin: 0 auto;
  padding: 36px 32px 0 112px;
}
.sk-host {
  position: relative;
}
@media (max-width: 760px) {
  .sk-paper {
    padding: 24px 16px 0 88px;
  }
}
</style>
