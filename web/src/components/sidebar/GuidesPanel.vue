<script setup lang="ts">
import { mdiRefresh } from '@mdi/js'
import { computed, onMounted, ref } from 'vue'
import { api, describeError } from '../../api/http'
import type { GuideSummary } from '../../api/types'
import { GENRES, genreLabel } from '../../core/labels'
import { useEditorStore } from '../../stores/editor'
import PanelHeader from '../PanelHeader.vue'
import StateBlock from '../StateBlock.vue'

const editor = useEditorStore()
const items = ref<GuideSummary[]>([])
const loading = ref(false)
const error = ref('')
const loaded = ref(false)
const genre = ref<string | null>(null)

const currentGenre = computed(() => {
  const t = editor.activeTab
  return t?.type === 'article' ? (editor.sessions.get(t.id)?.meta.genre ?? null) : null
})

const filtered = computed(() => (genre.value ? items.value.filter((g) => g.genres.includes(genre.value!)) : items.value))
const genresInUse = computed(() => GENRES.filter((g) => items.value.some((it) => it.genres.includes(g.value))))

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = await api.listGuides()
    loaded.value = true
  } catch (e) {
    error.value = describeError(e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (currentGenre.value) genre.value = currentGenre.value
  void load()
})
</script>

<template>
  <div class="sk-guides">
    <PanelHeader title="写作指引">
      <v-btn :icon="mdiRefresh" size="small" density="comfortable" aria-label="刷新" :loading="loading" @click="load" />
    </PanelHeader>

    <div v-if="loaded && items.length" class="pa-3 pb-1">
      <v-chip-group v-model="genre" selected-class="text-primary" column aria-label="按文体筛选">
        <v-chip :value="null" size="small" variant="outlined" filter>全部</v-chip>
        <v-chip v-for="g in genresInUse" :key="g.value" :value="g.value" size="small" variant="outlined" filter>
          {{ g.title }}
        </v-chip>
      </v-chip-group>
    </div>

    <StateBlock v-if="loading && !loaded" state="loading" text="正在读取写作指引" />
    <StateBlock v-else-if="error" state="error" :text="`指引读取失败：${error}`">
      <v-btn variant="outlined" size="small" @click="load">重新读取</v-btn>
    </StateBlock>
    <StateBlock v-else-if="loaded && items.length === 0" state="empty" text="后端没有内置写作指引。指引语料放在 internal/guide/data 下，重新构建后端后出现在这里。" />
    <StateBlock
      v-else-if="loaded && filtered.length === 0"
      state="empty"
      :text="`没有适用于“${genreLabel(genre ?? '')}”的指引。`"
    >
      <v-btn variant="outlined" size="small" @click="genre = null">看全部指引</v-btn>
    </StateBlock>

    <ul v-else class="sk-guide-list">
      <li v-for="g in filtered" :key="g.id">
        <button
          type="button"
          class="sk-guide"
          :class="{ 'sk-guide-active': editor.activeTab?.type === 'guide' && editor.activeTab.id === g.id }"
          @click="editor.openGuide(g.id)"
        >
          <span class="sk-guide-title">{{ g.title }}</span>
          <span class="sk-muted text-caption">{{ g.genres.map(genreLabel).join('、') }}</span>
        </button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.sk-guides {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.sk-guide-list {
  list-style: none;
  margin: 0;
  padding: 4px 0;
  overflow: auto;
  flex: 1;
}
.sk-guide {
  width: 100%;
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
.sk-guide:hover {
  background-color: rgba(var(--v-theme-on-surface), var(--v-hover-opacity));
}
.sk-guide-active {
  background-color: rgba(var(--v-theme-primary), 0.14);
}
.sk-guide-title {
  font-family: var(--sk-font-body);
  font-size: 14px;
}
</style>
