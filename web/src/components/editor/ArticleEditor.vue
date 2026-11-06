<script setup lang="ts">
import { mdiConsoleLine, mdiExportVariant } from '@mdi/js'
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import type { ParagraphKind } from '../../api/types'
import { ws } from '../../api/ws'
import { DESC_KINDS, PARAGRAPH_KINDS, kindLabel } from '../../core/labels'
import { exportArticle } from '../../editor/exporter'
import { blockedReason, degradeNote, planShiftEnter, prepare, type ActionInput } from '../../editor/scaffold'
import type { ArticleSession } from '../../editor/session'
import { useAnalysisStore } from '../../stores/analysis'
import { useEditorStore } from '../../stores/editor'
import { useNotifyStore } from '../../stores/notify'
import { useSuggestionStore } from '../../stores/suggestions'
import { useUiStore } from '../../stores/ui'
import ArticleHeader from './ArticleHeader.vue'
import CommandPalette, { type EditorUtility } from './CommandPalette.vue'
import HoverCard from './HoverCard.vue'
import RelationOverlay from './RelationOverlay.vue'
import ContextMenu, { type ContextAction } from '../ContextMenu.vue'
import SearchPanel from './SearchPanel.vue'
import { toggleFullscreen } from '../../core/fullscreen'

const props = defineProps<{ session: ArticleSession; active: boolean }>()

const analysis = useAnalysisStore()
const sg = useSuggestionStore()
const ui = useUiStore()
const editor = useEditorStore()
const notify = useNotifyStore()

const host = ref<HTMLElement | null>(null)
const paper = ref<HTMLElement | null>(null)
const s = props.session
const layers = s.ui.layers
const searchOpen = ref(false)
const searchReplace = ref(false)
const searchPanel = ref<InstanceType<typeof SearchPanel> | null>(null)
const organizeOpen = ref(false)
const organizing = ref(false)
const organizeError = ref('')
const organized = ref<{ key: string; index: number; before: ParagraphKind; after: ParagraphKind; role: string }[]>([])
let organizeSnapshot = s.doc
let organizeRun = 0

async function openSearch(replace = false) {
  palette.value = null
  searchReplace.value = replace
  searchOpen.value = true
  await nextTick()
  await searchPanel.value?.focus()
}

async function organizeTypes() {
  if (organizing.value) return
  const run = ++organizeRun
  for (const task of ws.tasks.items.filter((item) => item.articleId === s.articleId && item.kind === 'inference')) task.open = undefined
  organizeOpen.value = false
  organizing.value = true
  organizeError.value = ''
  organized.value = []
  organizeSnapshot = s.doc
  ui.openPanel('tasks')
  try {
    const result = await analysis.inferStructure(s, () => {
      editor.activate({ type: 'article', id: s.articleId })
      organizeOpen.value = true
    })
    if (run !== organizeRun) return
    if (!organizeSnapshot.eq(s.doc)) throw new Error('正文或段落类型已变化，请重新整理')
    const roles = new Map(result.paragraphs.map((paragraph) => [paragraph.paragraphId, paragraph]))
    const rows = s.localRows()
    organized.value = rows.map((row, index) => {
      const id = s.sync.idOf(row.key)
      const role = id === null ? undefined : roles.get(id)
      if (!role) throw new Error(`AI 没有返回第 ${index + 1} 段的类型，请重新整理`)
      return { key: row.key, index, before: row.kind, after: role.detectedKind, role: role.role }
    })
    notify.push('段落类型推断完成，可查看并应用。', { action: { label: '查看结果', run: () => { editor.activate({ type: 'article', id: s.articleId }); organizeOpen.value = true } } })
  } catch (error) {
    if (run !== organizeRun) return
    organizeError.value = error instanceof Error ? error.message : String(error)
  } finally {
    if (run === organizeRun) organizing.value = false
  }
}

function applyOrganizedTypes() {
  if (!organizeSnapshot.eq(s.doc)) {
    organizeError.value = '正文或段落类型已变化，请重新整理'
    return
  }
  s.applyKinds(new Map(organized.value.map((row) => [row.key, row.after])))
  organizeOpen.value = false
  notify.push('已统一整理段落类型。Ctrl+Z 可撤销。')
  s.focus()
}

function closeOrganize() {
  organizeOpen.value = false
  s.focus()
}

function cancelOrganize() {
  const task = ws.tasks.active.find((item) => item.articleId === s.articleId && item.kind === 'inference')
  task?.cancel()
}

function runUtility(action: EditorUtility) {
  palette.value = null
  switch (action) {
    case 'find': void openSearch(); break
    case 'replace': void openSearch(true); break
    case 'fullscreen': void toggleFullscreen().catch(() => notify.error('浏览器没有进入全屏，请使用浏览器的全屏菜单。')); break
    case 'organize': void organizeTypes(); break
    case 'analyze-current': analyzeCurrent(); s.focus(); break
    case 'analyze-all': analyzeAll(); s.focus(); break
    case 'settings': editor.openSettings(); break
  }
}
const contextOpen = ref(false)
const contextAt = ref<[number, number]>([0, 0])

async function clipboard(action: 'copy' | 'cut' | 'paste') {
  try {
    if (action === 'paste') await s.pasteText()
    else await s.copySelection(action === 'cut')
  } catch (error) {
    notify.error(`剪贴板操作失败：${error instanceof Error ? error.message : String(error)}。可使用 Ctrl+C / Ctrl+X / Ctrl+V。`)
  }
}

const contextActions = computed<ContextAction[]>(() => {
  void s.ui.revision
  const cursor = s.ui.cursor
  return [
    { title: '撤销', hint: 'Ctrl+Z', disabled: !s.canUndo(), run: () => s.undo() },
    { title: '重做', hint: 'Ctrl+Y', disabled: !s.canRedo(), run: () => s.redo() },
    { title: '剪切', hint: 'Ctrl+X', disabled: !s.hasSelection, run: () => { void clipboard('cut') } },
    { title: '复制', hint: 'Ctrl+C', disabled: !s.hasSelection, run: () => { void clipboard('copy') } },
    { title: '粘贴', hint: 'Ctrl+V', run: () => { void clipboard('paste') } },
    { title: '全选正文', hint: 'Ctrl+A', run: () => s.selectAll() },
    { title: '命令面板', hint: 'Ctrl+K', run: () => openPalette() },
    { title: '分析本段', disabled: analyzing.value || !cursor, run: analyzeCurrent },
    { title: '本段类型 / 新建段落', hint: 'Alt+T', disabled: !cursor, run: () => {
      if (cursor) kindMenu.value = { key: cursor.key, target: contextAt.value, mode: 'set' }
    } },
    { title: '上移一段', disabled: !cursor || cursor.index === 0, run: () => { s.moveCurrent(-1) } },
    { title: '下移一段', disabled: !cursor || cursor.index === s.doc.childCount - 1, run: () => { s.moveCurrent(1) } },
    { title: '删除本段', danger: true, disabled: !cursor, run: () => { if (cursor) s.deleteParagraph(cursor.key) } },
  ]
})

function restoreContextFocus() {
  if (!palette.value && !kindMenu.value) s.focus()
}

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
    onContextMenu: (at) => {
      contextAt.value = at
      contextOpen.value = true
    },
    onSearch: (replace) => { void openSearch(replace) },
  })
  sg.refreshEditor(s.articleId)
})

watch(
  () => props.active,
  async (v) => {
    if (v) {
      await nextTick()
      s.refresh()
    } else {
      contextOpen.value = false
      palette.value = null
      searchOpen.value = false
      kindMenu.value = null
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
const analyzing = computed(() => st.value.preparing > 0 || st.value.inflight > 0 || st.value.articleLoading)

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
    notify.error('有改动没有保存成功，导出会缺内容。先处理状态栏中的保存失败。')
  }
}

const layerButtons = [
  { key: 'desc', label: '描写', hint: '半句描写类型高亮' },
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
            <v-btn
              v-bind="tp"
              type="button"
              size="small"
              :variant="layers[b.key] ? 'tonal' : 'text'"
              :color="layers[b.key] ? 'primary' : undefined"
              :aria-pressed="layers[b.key]"
              @click="layers[b.key] = !layers[b.key]"
            >
              {{ b.label }}
            </v-btn>
          </template>
        </v-tooltip>
      </span>

      <span class="sk-tb-sep" />

      <span class="sk-tb-group">
        <v-btn size="small" :disabled="analyzing" @click="analyzeCurrent">分析本段</v-btn>
        <v-btn size="small" :disabled="analyzing" @click="analyzeAll">分析全文</v-btn>
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

    <div class="sk-scroll">
      <Transition name="sk-menu">
        <SearchPanel v-if="searchOpen && active" ref="searchPanel" v-model:replace="searchReplace" :session="s" @close="searchOpen = false; s.focus()" />
      </Transition>
      <div ref="paper" class="sk-paper" :style="{ fontSize: `${18 * ui.editorZoom / 100}px`, '--sk-paper-width': `${ui.layout.editorWidth}px` }" :class="{ 'sk-genre-poetry': s.meta.genre === 'poetry' }">
        <ArticleHeader :session="s" @enter-body="s.focus()" />
        <div ref="host" class="sk-host" />
        <RelationOverlay :session="s" :paper="paper" />
      </div>
    </div>

    <footer class="sk-editor-footer bg-sk-panel" aria-label="编辑器图例与排版">
      <div class="sk-footer-legend" :class="{ 'sk-muted': !layers.desc }" aria-label="描写类型图例">
        <span class="sk-muted">图例</span>
        <span v-for="item in legend" :key="item.value" :class="`sk-desc-${item.value}`">{{ item.title }}</span>
      </div>
      <v-menu location="top start">
        <template #activator="{ props: menuProps }"><v-btn v-bind="menuProps" size="small">标记图例</v-btn></template>
        <v-card class="pa-3" max-width="320">
          <div class="sk-legend-row"><span class="sk-swatch sk-mk-theme">点题</span>方框：点题字眼</div>
          <div class="sk-legend-row"><span class="sk-swatch sk-mk-keyword">关键</span>实线：关键词</div>
          <div class="sk-legend-row"><span class="sk-swatch sk-mk-repeat">重复</span>波浪线：重复用词</div>
          <div class="sk-legend-row"><span class="sk-swatch sk-mk-pronoun">他</span>虚线：代词</div>
          <div class="sk-legend-row"><span class="sk-swatch sk-mk-callback">呼应</span>双线：前后呼应</div>
          <div class="sk-legend-row"><span class="sk-swatch sk-diag sk-diag-warning">诊断</span>点线：诊断</div>
          <div class="sk-legend-row"><span class="sk-swatch sk-cmd">[指令]</span>给 Scaffolder 的指令</div>
        </v-card>
      </v-menu>
      <v-menu location="top end" :close-on-content-click="false">
        <template #activator="{ props: menuProps }"><v-btn v-bind="menuProps" size="small">左右留白</v-btn></template>
        <v-card class="pa-4" width="300" max-width="calc(100vw - 16px)">
          <v-slider v-model="ui.layout.editorWidth" label="稿面宽度" :min="560" :max="1600" :step="40" thumb-label="always" hide-details class="mt-5" />
          <p class="text-caption sk-muted mt-2">稿面始终居中；加宽会同时缩小两侧留白。窄屏自动适配。</p>
          <v-btn size="small" @click="ui.layout.editorWidth = 880">恢复默认</v-btn>
        </v-card>
      </v-menu>
    </footer>

    <ContextMenu v-model="contextOpen" :at="contextAt" label="正文操作" :actions="contextActions" @closed="restoreContextFocus" />
    <HoverCard v-if="active" :session="s" :disabled="!!palette || contextOpen || !!kindMenu || organizeOpen" />

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
          <p v-if="s.ui.kindLocked" class="text-caption px-4 pt-2" role="status">类型推断中，取消后台任务后可手动切换。</p>
          <v-list density="compact" class="py-0">
            <v-list-item
              v-for="k in PARAGRAPH_KINDS"
              :key="k.value"
              :title="k.title"
              :subtitle="k.hint"
              :active="kindMenuNode?.node.attrs.kind === k.value"
              :disabled="s.ui.kindLocked"
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
      :reason="palette.reason"
      :busy="analyzing"
      @run="runAction"
      @set-kind="(k) => { if (!s.ui.kindLocked) { s.setKind(s.ui.cursor!.key, k); closePalette(); notify.push(`本段已设为${kindLabel(k)}。`) } }"
      @close="closePalette"
      @utility="runUtility"
    />

    <v-dialog :model-value="organizeOpen" max-width="760" scrollable @update:model-value="!$event && closeOrganize()">
      <v-card title="AI 整理段落类型">
        <v-card-text>
          <p v-if="organizing" class="py-4" role="status">正在后台结合全文推断每段的类型，手动切换类型已锁定。可关闭此窗口继续写作，或取消任务后手动切换。</p>
          <v-alert v-else-if="organizeError" type="error" variant="tonal">{{ organizeError }}</v-alert>
          <template v-else>
            <p class="text-body-2 sk-muted mb-4">检查每段的推断结果，可在应用前调整。只修改类型，正文保持原样。</p>
            <v-list lines="two" class="pa-0">
              <v-list-item v-for="row in organized" :key="row.key" :title="`第 ${row.index + 1} 段 · 原类型：${kindLabel(row.before)}`" :subtitle="row.role" class="px-0 py-2">
                <template #append><v-select v-model="row.after" :items="PARAGRAPH_KINDS" item-value="value" item-title="title" :label="`第 ${row.index + 1} 段类型`" width="140" hide-details class="ml-4" /></template>
              </v-list-item>
            </v-list>
          </template>
        </v-card-text>
        <v-card-actions class="flex-wrap"><v-btn :disabled="organizing" @click="organizeTypes">重新推断</v-btn><v-spacer /><v-btn v-if="organizing" @click="cancelOrganize">取消推断</v-btn><v-btn @click="closeOrganize">{{ organizing ? '继续后台运行' : '关闭' }}</v-btn><v-btn variant="flat" color="primary" :disabled="organizing || !!organizeError || !organized.length" @click="applyOrganizedTypes">应用类型</v-btn></v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<style scoped>
.sk-article {
  position: relative;
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
  min-height: 48px;
  padding: 4px 8px;
  border-bottom: 1px solid var(--sk-line);
  font-size: 13px;
  flex-wrap: wrap;
}
.sk-tb-group {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-wrap: wrap;
  min-width: 0;
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
.sk-scroll {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: auto;
  overflow-y: scroll;
  scrollbar-gutter: stable;
}
.sk-paper {
  position: relative;
  width: min(var(--sk-paper-width), calc(100% - 192px));
  margin: 0 auto;
  padding: 32px 0 0;
}
.sk-editor-footer {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  flex: none;
  gap: 4px 12px;
  padding: 4px 12px;
  border-top: 1px solid var(--sk-line);
  font-size: 12px;
}
.sk-footer-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 12px;
  flex: 1;
}
.sk-host {
  position: relative;
}
@media (max-width: 760px) {
  .sk-paper {
    width: auto;
    margin: 0 16px 0 88px;
    padding: 24px 0 0;
  }
}
</style>
