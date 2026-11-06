<script setup lang="ts">
import { mdiDeleteOutline, mdiNoteTextOutline, mdiRefresh, mdiUploadOutline } from '@mdi/js'
import { computed, ref, watch } from 'vue'
import { api, describeError } from '../../api/http'
import type { Material } from '../../api/types'
import { useDialogStore } from '../../stores/dialogs'
import { useEditorStore } from '../../stores/editor'
import { useNotifyStore } from '../../stores/notify'
import { useWorkspaceStore } from '../../stores/workspace'
import PanelHeader from '../PanelHeader.vue'
import StateBlock from '../StateBlock.vue'

// 素材 = 喂给 Scaffolder 的参考（范文、资料）。归属：全局 / 文集 / 文章。

type Scope = 'all' | 'collection' | 'article'

const editor = useEditorStore()
const workspace = useWorkspaceStore()
const dialogs = useDialogStore()
const notify = useNotifyStore()

const scope = ref<Scope>('all')
const items = ref<Material[]>([])
const loading = ref(false)
const error = ref('')
const expanded = ref<number | null>(null)
const uploading = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

const pasteOpen = ref(false)
const pasteName = ref('')
const pasteContent = ref('')
const pasteOwner = ref<Scope>('all')
const pasteSaving = ref(false)

const article = computed(() => {
  const t = editor.activeTab
  return t?.type === 'article' ? (editor.sessions.get(t.id)?.meta ?? null) : null
})
const collectionId = computed(() => article.value?.collectionId ?? null)

const scopes = computed(() => [
  { value: 'all', title: '全部素材' },
  { value: 'collection', title: '当前文章所在文集', props: { disabled: collectionId.value === null } },
  { value: 'article', title: '当前文章', props: { disabled: !article.value } },
])
const owners = computed(() => [
  { value: 'all', title: '全局（所有文章都能用）' },
  { value: 'collection', title: '当前文集', props: { disabled: collectionId.value === null } },
  { value: 'article', title: '当前文章', props: { disabled: !article.value } },
])

function ownerOf(s: Scope) {
  if (s === 'collection' && collectionId.value !== null) return { collectionId: collectionId.value }
  if (s === 'article' && article.value) return { articleId: article.value.id }
  return {}
}

function ownerLabel(m: Material) {
  if (m.articleId) return '文章'
  if (m.collectionId) return workspace.collections.find((c) => c.id === m.collectionId)?.name ?? '文集'
  return '全局'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const q = ownerOf(scope.value)
    items.value = await api.listMaterials(q)
  } catch (e) {
    error.value = describeError(e)
  } finally {
    loading.value = false
  }
}

watch(scope, load, { immediate: true })
watch(article, () => {
  if (scope.value !== 'all') {
    if (!article.value || (scope.value === 'collection' && collectionId.value === null)) scope.value = 'all'
    else void load()
  }
})

function pickFile() {
  fileInput.value?.click()
}

async function onFile(e: Event) {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  input.value = ''
  if (files.length === 0) return
  uploading.value = true
  try {
    for (const f of files) {
      if (!/\.(txt|md)$/i.test(f.name)) {
        notify.error(`“${f.name}”不是 .txt 或 .md，已跳过。`)
        continue
      }
      await api.uploadMaterial(f, ownerOf(scope.value === 'all' ? 'all' : scope.value))
    }
    await load()
  } catch (err) {
    notify.error(`上传失败：${describeError(err)}`)
  } finally {
    uploading.value = false
  }
}

function openPaste() {
  pasteName.value = ''
  pasteContent.value = ''
  pasteOwner.value = scope.value
  pasteOpen.value = true
}

async function savePaste() {
  if (!pasteName.value.trim() || !pasteContent.value.trim()) return
  pasteSaving.value = true
  try {
    await api.createMaterial({ name: pasteName.value.trim(), content: pasteContent.value, ...ownerOf(pasteOwner.value) })
    pasteOpen.value = false
    await load()
  } catch (e) {
    notify.error(`没有保存：${describeError(e)}`)
  } finally {
    pasteSaving.value = false
  }
}

async function remove(m: Material) {
  const ok = await dialogs.confirm({
    title: `删除素材“${m.name}”`,
    text: 'Scaffolder 以后不会再参考它。',
    confirm: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await api.deleteMaterial(m.id)
    items.value = items.value.filter((x) => x.id !== m.id)
  } catch (e) {
    notify.error(`删除失败：${describeError(e)}`)
  }
}
</script>

<template>
  <div class="sk-materials">
    <PanelHeader title="素材">
      <v-btn :icon="mdiNoteTextOutline" size="small" density="comfortable" aria-label="粘贴新建素材" title="粘贴文字新建" @click="openPaste" />
      <v-btn :icon="mdiUploadOutline" size="small" density="comfortable" aria-label="上传 txt 或 md" title="上传 .txt / .md" :loading="uploading" @click="pickFile" />
      <v-btn :icon="mdiRefresh" size="small" density="comfortable" aria-label="刷新" :loading="loading" @click="load" />
    </PanelHeader>
    <input ref="fileInput" type="file" accept=".txt,.md,text/plain,text/markdown" multiple hidden @change="onFile" />

    <div class="pa-3 pb-1">
      <v-select v-model="scope" :items="scopes" label="显示" />
      <p class="sk-muted text-caption mt-2 mb-0">上传和粘贴的素材归入当前选择的范围；选“全部”时归为全局。</p>
    </div>

    <StateBlock v-if="loading && items.length === 0" state="loading" text="正在读取素材" />
    <StateBlock v-else-if="error" state="error" :text="`素材读取失败：${error}`">
      <v-btn variant="outlined" size="small" @click="load">重新读取</v-btn>
    </StateBlock>
    <StateBlock
      v-else-if="items.length === 0"
      state="empty"
      text="还没有素材。把范文、资料、自己的旧作导进来，Scaffolder 生成时会检索相关片段作参考。"
    >
      <v-btn variant="outlined" size="small" @click="pickFile">上传 .txt / .md</v-btn>
      <v-btn variant="outlined" size="small" @click="openPaste">粘贴文字</v-btn>
    </StateBlock>

    <ul v-else class="sk-mat-list">
      <li v-for="m in items" :key="m.id" class="sk-mat">
        <div class="sk-mat-row">
          <button
            type="button"
            class="sk-mat-name"
            :aria-expanded="expanded === m.id"
            @click="expanded = expanded === m.id ? null : m.id"
          >
            <span class="sk-mat-title">{{ m.name }}</span>
            <span class="sk-muted text-caption">{{ ownerLabel(m) }} · {{ m.content.length }} 字</span>
          </button>
          <v-btn :icon="mdiDeleteOutline" size="x-small" density="comfortable" :aria-label="`删除素材 ${m.name}`" @click="remove(m)" />
        </div>
        <pre v-if="expanded === m.id" class="sk-mat-preview">{{ m.content.length > 1200 ? m.content.slice(0, 1200) + '…' : m.content }}</pre>
      </li>
    </ul>

    <v-dialog v-model="pasteOpen" max-width="560">
      <v-card title="粘贴新建素材">
        <v-card-text class="d-flex flex-column ga-3">
          <v-text-field v-model="pasteName" label="名称" autofocus />
          <v-textarea v-model="pasteContent" label="内容" rows="8" />
          <v-select v-model="pasteOwner" :items="owners" label="归属" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="pasteOpen = false">取消</v-btn>
          <v-btn color="primary" variant="flat" :disabled="!pasteName.trim() || !pasteContent.trim()" :loading="pasteSaving" @click="savePaste">保存素材</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<style scoped>
.sk-materials {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.sk-mat-list {
  list-style: none;
  margin: 0;
  padding: 4px 0;
  overflow: auto;
  flex: 1;
}
.sk-mat-row {
  display: flex;
  align-items: center;
  padding-right: 4px;
}
.sk-mat-name {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  padding: 6px 12px;
  background: none;
  border: 0;
  color: inherit;
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.sk-mat-name:hover {
  background-color: rgba(var(--v-theme-on-surface), var(--v-hover-opacity));
}
.sk-mat-title {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sk-mat-preview {
  margin: 0 12px 8px;
  padding: 8px;
  font-family: var(--sk-font-body);
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  max-height: 260px;
  overflow: auto;
  border: 1px solid var(--sk-line);
  border-radius: 3px;
}
</style>
