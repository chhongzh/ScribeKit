<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api, describeError } from '../../api/http'
import { genreLabel } from '../../core/labels'
import { useEditorStore } from '../../stores/editor'
import { useWorkspaceStore } from '../../stores/workspace'
import StateBlock from '../StateBlock.vue'
import ExpandableTextarea from '../ExpandableTextarea.vue'

// 文集总纲：整个文集共用的约束、风格、规则，Scaffolder 生成时会读取。

const props = defineProps<{ collectionId: number }>()
const editor = useEditorStore()
const workspace = useWorkspaceStore()

const detail = computed(() => editor.outlines[props.collectionId] ?? null)
const state = computed(() => editor.open[`outline:${props.collectionId}`])
const name = ref('')
const outline = ref('')
const saveState = ref<'idle' | 'saving' | 'error'>('idle')
const saveError = ref('')
let timer: ReturnType<typeof setTimeout> | null = null
let loaded = false

watch(
  detail,
  (d) => {
    if (d && !loaded) {
      name.value = d.name
      outline.value = d.outline
      loaded = true
    }
  },
  { immediate: true },
)

function queue() {
  if (!loaded) return
  saveState.value = 'saving'
  if (timer) clearTimeout(timer)
  timer = setTimeout(save, 800)
}

async function save() {
  timer = null
  try {
    await api.updateCollection(props.collectionId, { name: name.value.trim() || '未命名文集', outline: outline.value })
    saveState.value = 'idle'
    const c = workspace.collections.find((x) => x.id === props.collectionId)
    if (c) c.name = name.value.trim() || '未命名文集'
    if (detail.value) detail.value.name = name.value
  } catch (e) {
    saveState.value = 'error'
    saveError.value = describeError(e)
  }
}

watch([name, outline], queue)
onBeforeUnmount(() => {
  if (timer) {
    clearTimeout(timer)
    void save()
  }
})
</script>

<template>
  <div class="sk-outline-wrap">
    <StateBlock v-if="state?.loading && !detail" state="loading" text="正在读取总纲" />
    <StateBlock v-else-if="state?.error" state="error" :text="`总纲读取失败：${state.error}`">
      <v-btn variant="outlined" size="small" @click="editor.openOutline(collectionId)">再试一次</v-btn>
    </StateBlock>
    <div v-else-if="detail" class="sk-outline">
      <v-text-field v-model="name" label="文集名称" class="mb-4" aria-label="文集名称" />
      <p class="sk-muted text-body-2 mb-4">
        总纲是这个文集里所有文章共用的约束：立意、人物设定、语言风格、不能写的东西。Scaffolder 为这个文集里的任何一篇生成内容时都会先读它。可以不写。
      </p>
      <ExpandableTextarea
        v-model="outline"
        label="总纲"
        :rows="12"
        auto-grow
        aria-label="总纲"
        placeholder="例如：主人公是初三学生，第一人称；语言朴素，不用华丽辞藻；每篇都要落到“成长要付出代价”上。"
      />
      <p class="sk-muted text-caption" role="status">
        <template v-if="saveState === 'saving'">保存中</template>
        <span v-else-if="saveState === 'error'" class="text-error">没有保存：{{ saveError }}
          <button type="button" class="sk-plain text-decoration-underline" @click="save">重试</button>
        </span>
        <template v-else>已保存</template>
      </p>
      <h3 class="sk-outline-sub">文集里的文章</h3>
      <p v-if="detail.articles.length === 0" class="sk-muted text-body-2">还没有文章。在资源管理器里文集的菜单中选“在此新建文章”。</p>
      <ul v-else class="sk-outline-articles">
        <li v-for="a in detail.articles" :key="a.id">
          <button type="button" class="sk-plain text-decoration-underline" @click="editor.openArticle(a.id)">{{ a.title || '未命名' }}</button>
          <span class="sk-muted text-caption"> · {{ genreLabel(a.genre) }} · {{ a.charCount }} 字</span>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.sk-outline-wrap {
  height: 100%;
  overflow: auto;
}
.sk-outline {
  max-width: 44em;
  margin: 0 auto;
  padding: 36px 32px;
}
.sk-outline-name {
  width: 100%;
  font-family: var(--sk-font-body);
  font-size: 26px;
  font-weight: 600;
  border: 0;
  background: none;
  color: rgb(var(--v-theme-on-surface));
  margin-bottom: 8px;
}
.sk-outline-name:focus-visible {
  outline: none;
  box-shadow: 0 1px 0 rgb(var(--v-theme-primary));
}
.sk-outline-text {
  display: block;
  width: 100%;
  min-height: 40vh;
  resize: vertical;
  border: 1px solid var(--sk-line);
  border-radius: 3px;
  padding: 12px 14px;
  font-family: var(--sk-font-body);
  font-size: 16px;
  line-height: 1.9;
  color: rgb(var(--v-theme-on-surface));
  background: none;
}
.sk-outline-text::placeholder {
  color: var(--sk-muted);
}
.sk-outline-sub {
  font-size: 14px;
  margin: 28px 0 8px;
}
.sk-outline-articles {
  padding-left: 18px;
  line-height: 2;
  font-family: var(--sk-font-body);
}
</style>
