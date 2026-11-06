<script setup lang="ts">
import {
  mdiChevronDown,
  mdiChevronRight,
  mdiDotsHorizontal,
  mdiFileDocumentOutline,
  mdiFilePlusOutline,
  mdiFolderOutline,
  mdiFolderPlusOutline,
  mdiRefresh,
} from '@mdi/js'
import { computed, reactive, ref } from 'vue'
import { api, describeError } from '../../api/http'
import type { ArticleSummary, Collection } from '../../api/types'
import { genreLabel } from '../../core/labels'
import { exportArticle } from '../../editor/exporter'
import { useDialogStore } from '../../stores/dialogs'
import { useEditorStore } from '../../stores/editor'
import { useNotifyStore } from '../../stores/notify'
import { useWorkspaceStore } from '../../stores/workspace'
import PanelHeader from '../PanelHeader.vue'
import StateBlock from '../StateBlock.vue'
import NewArticleDialog from './NewArticleDialog.vue'

const workspace = useWorkspaceStore()
const editor = useEditorStore()
const dialogs = useDialogStore()
const notify = useNotifyStore()

const expanded = reactive<Record<number, boolean>>({})
const looseOpen = ref(true)
const newArticleOpen = ref(false)
const newArticleCollection = ref<number | null>(null)

const empty = computed(() => workspace.loaded && workspace.collections.length === 0 && workspace.loose.length === 0)
const activeArticle = computed(() => (editor.activeTab?.type === 'article' ? editor.activeTab.id : null))

function isOpen(c: Collection) {
  return expanded[c.id] ?? true
}

function newArticle(collectionId: number | null) {
  newArticleCollection.value = collectionId
  newArticleOpen.value = true
}

async function newCollection() {
  const name = await dialogs.prompt({ title: '新建文集', label: '文集名称', confirm: '建好' })
  if (!name) return
  try {
    const c = await workspace.createCollection(name)
    expanded[c.id] = true
  } catch (e) {
    notify.error(`文集没有建成：${describeError(e)}`)
  }
}

async function renameCollection(c: Collection) {
  const name = await dialogs.prompt({ title: '重命名文集', label: '文集名称', value: c.name, confirm: '改名' })
  if (!name || name === c.name) return
  try {
    await workspace.renameCollection(c.id, name)
  } catch (e) {
    notify.error(`改名失败：${describeError(e)}`)
  }
}

async function deleteCollection(c: Collection) {
  const ok = await dialogs.confirm({
    title: `删除文集“${c.name}”`,
    text: '文集里的文章不会被删除，会变成散篇；总纲会随文集一起删除。',
    confirm: '删除文集',
    danger: true,
  })
  if (!ok) return
  try {
    await workspace.deleteCollection(c.id)
    editor.forget({ type: 'outline', id: c.id })
  } catch (e) {
    notify.error(`删除失败：${describeError(e)}`)
  }
}

async function renameArticle(a: ArticleSummary) {
  const title = await dialogs.prompt({ title: '重命名文章', label: '标题', value: a.title, confirm: '改名' })
  if (!title || title === a.title) return
  try {
    await workspace.renameArticle(a.id, title)
    const s = editor.sessions.get(a.id)
    if (s) s.meta.title = title
  } catch (e) {
    notify.error(`改名失败：${describeError(e)}`)
  }
}

async function deleteArticle(a: ArticleSummary) {
  const ok = await dialogs.confirm({
    title: `删除“${a.title || '未命名'}”`,
    text: '文章的全部段落、待处理的建议会一起删除，不能恢复。',
    confirm: '删除文章',
    danger: true,
  })
  if (!ok) return
  try {
    await workspace.deleteArticle(a.id)
    editor.forget({ type: 'article', id: a.id })
  } catch (e) {
    notify.error(`删除失败：${describeError(e)}`)
  }
}

async function moveArticle(a: ArticleSummary, collectionId: number | null) {
  try {
    await api.updateArticle(a.id, { collectionId })
    await workspace.refresh()
    const s = editor.sessions.get(a.id)
    if (s) s.meta.collectionId = collectionId
  } catch (e) {
    notify.error(`移动失败：${describeError(e)}`)
  }
}

function doExport(a: ArticleSummary, format: 'md' | 'docx') {
  exportArticle(a.id, format, editor.sessions.get(a.id)).catch(() =>
    notify.error('有改动没有保存成功，导出会缺内容。先处理编辑器顶部的保存失败。'),
  )
}

/** 方向键在树节点间移动焦点。 */
function onTreeKey(e: KeyboardEvent) {
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
  const rows = Array.from((e.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('.sk-row-main'))
  const i = rows.indexOf(document.activeElement as HTMLElement)
  const next = rows[e.key === 'ArrowDown' ? i + 1 : i - 1]
  if (next) {
    e.preventDefault()
    next.focus()
  }
}
</script>

<template>
  <div class="sk-explorer">
    <PanelHeader title="资源管理器">
      <v-btn :icon="mdiFilePlusOutline" size="small" density="comfortable" aria-label="新建散篇文章" title="新建文章（散篇）" @click="newArticle(null)" />
      <v-btn :icon="mdiFolderPlusOutline" size="small" density="comfortable" aria-label="新建文集" title="新建文集" @click="newCollection" />
      <v-btn :icon="mdiRefresh" size="small" density="comfortable" aria-label="刷新" title="从后端重新读取" :loading="workspace.loading" @click="workspace.refresh()" />
    </PanelHeader>

    <StateBlock v-if="!workspace.loaded && workspace.loading" state="loading" text="正在读取文集和文章" />
    <StateBlock v-else-if="workspace.error && !workspace.loaded" state="error" :text="workspace.error">
      <v-btn variant="outlined" size="small" @click="workspace.refresh()">重新读取</v-btn>
    </StateBlock>
    <StateBlock
      v-else-if="empty"
      state="empty"
      text="还没有文章。临时起意写一篇短文，直接新建文章；同一主题要写好几篇，先建一个文集，它们能互相看见。"
    >
      <v-btn variant="flat" color="primary" size="small" @click="newArticle(null)">新建文章</v-btn>
      <v-btn variant="outlined" size="small" @click="newCollection">新建文集</v-btn>
    </StateBlock>

    <div v-else class="sk-tree" role="tree" aria-label="文集与文章" @keydown="onTreeKey">
      <p v-if="workspace.error" class="text-error text-caption px-3" role="alert">刷新失败：{{ workspace.error }}</p>

      <div v-for="c in workspace.collections" :key="c.id" role="treeitem" :aria-expanded="isOpen(c)">
        <div class="sk-row">
          <button type="button" class="sk-row-main" @click="expanded[c.id] = !isOpen(c)">
            <v-icon :icon="isOpen(c) ? mdiChevronDown : mdiChevronRight" size="16" />
            <v-icon :icon="mdiFolderOutline" size="16" />
            <span class="sk-row-label">{{ c.name }}</span>
            <span class="sk-row-meta">{{ (workspace.articlesByCollection[c.id] ?? []).length }}</span>
          </button>
          <v-menu location="bottom end">
            <template #activator="{ props: mp }">
              <v-btn v-bind="mp" :icon="mdiDotsHorizontal" size="x-small" density="comfortable" :aria-label="`文集“${c.name}”的操作`" />
            </template>
            <v-list density="compact">
              <v-list-item title="打开总纲" @click="editor.openOutline(c.id)" />
              <v-list-item title="在此新建文章" @click="newArticle(c.id)" />
              <v-list-item title="重命名" @click="renameCollection(c)" />
              <v-list-item title="删除文集" base-color="error" @click="deleteCollection(c)" />
            </v-list>
          </v-menu>
        </div>
        <div v-if="isOpen(c)" role="group">
          <button type="button" class="sk-row-main sk-indent sk-outline-row" @click="editor.openOutline(c.id)">
            <span class="sk-row-label">总纲</span>
          </button>
          <div v-for="a in workspace.articlesByCollection[c.id] ?? []" :key="a.id" class="sk-row" role="treeitem">
            <button
              type="button"
              class="sk-row-main sk-indent"
              :class="{ 'sk-row-active': activeArticle === a.id }"
              @click="editor.openArticle(a.id)"
            >
              <v-icon :icon="mdiFileDocumentOutline" size="16" />
              <span class="sk-row-label">{{ a.title || '未命名' }}</span>
              <span class="sk-row-meta">{{ genreLabel(a.genre) }} · {{ a.charCount }} 字</span>
            </button>
            <v-menu location="bottom end">
              <template #activator="{ props: mp }">
                <v-btn v-bind="mp" :icon="mdiDotsHorizontal" size="x-small" density="comfortable" :aria-label="`文章“${a.title}”的操作`" />
              </template>
              <v-list density="compact">
                <v-list-item title="重命名" @click="renameArticle(a)" />
                <v-list-item title="导出 Markdown" @click="doExport(a, 'md')" />
                <v-list-item title="导出 Docx" @click="doExport(a, 'docx')" />
                <v-list-item title="移出文集（变成散篇）" @click="moveArticle(a, null)" />
                <v-list-item title="删除文章" base-color="error" @click="deleteArticle(a)" />
              </v-list>
            </v-menu>
          </div>
          <p v-if="(workspace.articlesByCollection[c.id] ?? []).length === 0" class="sk-muted text-caption sk-indent-text">
            这个文集还没有文章。
          </p>
        </div>
      </div>

      <div role="treeitem" :aria-expanded="looseOpen">
        <div class="sk-row">
          <button type="button" class="sk-row-main" @click="looseOpen = !looseOpen">
            <v-icon :icon="looseOpen ? mdiChevronDown : mdiChevronRight" size="16" />
            <span class="sk-row-label">散篇</span>
            <span class="sk-row-meta">{{ workspace.loose.length }}</span>
          </button>
        </div>
        <div v-if="looseOpen" role="group">
          <div v-for="a in workspace.loose" :key="a.id" class="sk-row" role="treeitem">
            <button
              type="button"
              class="sk-row-main sk-indent"
              :class="{ 'sk-row-active': activeArticle === a.id }"
              @click="editor.openArticle(a.id)"
            >
              <v-icon :icon="mdiFileDocumentOutline" size="16" />
              <span class="sk-row-label">{{ a.title || '未命名' }}</span>
              <span class="sk-row-meta">{{ genreLabel(a.genre) }} · {{ a.charCount }} 字</span>
            </button>
            <v-menu location="bottom end">
              <template #activator="{ props: mp }">
                <v-btn v-bind="mp" :icon="mdiDotsHorizontal" size="x-small" density="comfortable" :aria-label="`文章“${a.title}”的操作`" />
              </template>
              <v-list density="compact">
                <v-list-item title="重命名" @click="renameArticle(a)" />
                <v-list-item title="导出 Markdown" @click="doExport(a, 'md')" />
                <v-list-item title="导出 Docx" @click="doExport(a, 'docx')" />
                <v-list-item
                  v-for="c in workspace.collections"
                  :key="c.id"
                  :title="`移入文集“${c.name}”`"
                  @click="moveArticle(a, c.id)"
                />
                <v-list-item title="删除文章" base-color="error" @click="deleteArticle(a)" />
              </v-list>
            </v-menu>
          </div>
          <p v-if="workspace.loose.length === 0" class="sk-muted text-caption sk-indent-text">没有散篇。</p>
        </div>
      </div>
    </div>

    <NewArticleDialog v-model="newArticleOpen" :collection-id="newArticleCollection" />
  </div>
</template>

<style scoped>
.sk-explorer {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.sk-tree {
  flex: 1;
  overflow: auto;
  padding: 4px 0 16px;
  font-size: 13px;
}
.sk-row {
  display: flex;
  align-items: center;
  padding-right: 4px;
}
.sk-row-main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 8px;
  background: none;
  border: 0;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
  border-radius: 2px;
}
.sk-row-main:hover {
  background-color: rgba(var(--v-theme-on-surface), var(--v-hover-opacity));
}
.sk-row-main:focus-visible {
  outline-offset: -2px;
}
.sk-row-active {
  background-color: rgba(var(--v-theme-primary), 0.14);
}
.sk-indent {
  padding-left: 30px;
}
.sk-indent-text {
  padding: 2px 0 4px 30px;
  margin: 0;
}
.sk-outline-row {
  color: var(--sk-muted);
  font-family: var(--sk-font-body);
}
.sk-row-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sk-row-meta {
  flex: none;
  font-size: 11px;
  color: var(--sk-muted);
}
</style>
