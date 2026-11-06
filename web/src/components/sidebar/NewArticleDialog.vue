<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { describeError } from '../../api/http'
import type { Genre } from '../../api/types'
import { GENRES } from '../../core/labels'
import { useEditorStore } from '../../stores/editor'
import { useWorkspaceStore } from '../../stores/workspace'
import ExpandableTextarea from '../ExpandableTextarea.vue'

const props = defineProps<{ collectionId: number | null }>()
const open = defineModel<boolean>({ required: true })

const workspace = useWorkspaceStore()
const editor = useEditorStore()

const title = ref('')
const synopsis = ref('')
const genre = ref<Genre | null>(null)
const subgenre = ref('')
const collectionId = ref<number | null>(null)
const saving = ref(false)
const error = ref('')

const SUBGENRES: Partial<Record<Genre, string[]>> = {
  narrative: ['写人记叙文', '叙事记叙文', '写景记叙文', '应试记叙文'],
  argumentative: ['应试议论文', '立论文', '驳论文', '时评'],
  prose: ['写景散文', '抒情散文', '叙事散文'],
  poetry: ['现代诗', '散文诗'],
  essay: ['随笔', '杂感'],
  expository: ['事物说明文', '事理说明文'],
  practical: ['书信', '演讲稿', '倡议书'],
}

const subOptions = computed(() => (genre.value ? (SUBGENRES[genre.value] ?? []) : []))
const collections = computed(() => [
  { value: null, title: '不放进文集（散篇）' },
  ...workspace.collections.map((c) => ({ value: c.id, title: c.name })),
])
const valid = computed(() => title.value.trim() !== '' && synopsis.value.trim() !== '' && genre.value !== null)

watch(open, (v) => {
  if (v) {
    title.value = ''
    synopsis.value = ''
    genre.value = null
    subgenre.value = ''
    error.value = ''
    collectionId.value = props.collectionId
  }
})

async function submit() {
  if (!valid.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    const a = await workspace.createArticle({
      title: title.value.trim(),
      synopsis: synopsis.value.trim(),
      genre: genre.value!,
      subgenre: subgenre.value.trim(),
      collectionId: collectionId.value,
    })
    open.value = false
    await editor.openArticle(a.id)
  } catch (e) {
    error.value = `没有建成：${describeError(e)}`
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <v-dialog v-model="open" max-width="560">
    <v-card title="新建文章">
      <v-card-text class="d-flex flex-column ga-4">
        <p class="sk-muted text-body-2">
          先写下标题和梗概。梗概可以很乱，它是你自己的草稿，也是 Scaffolder 理解你想说什么的唯一依据。
        </p>
        <v-text-field v-model="title" label="标题" autofocus />
        <ExpandableTextarea v-model="synopsis" label="梗概" rows="4" auto-grow placeholder="想写什么、怎么开头、要落到哪个观点上，想到哪写到哪" />
        <div class="d-flex ga-3">
          <v-select v-model="genre" :items="GENRES" label="文体" class="flex-1-1" />
          <v-combobox v-model="subgenre" :items="subOptions" label="细分（可选）" class="flex-1-1" />
        </div>
        <v-select v-model="collectionId" :items="collections" label="归属" />
        <p v-if="error" class="text-error text-body-2" role="alert">{{ error }}</p>
      </v-card-text>
      <v-card-actions>
        <span v-if="!valid" class="sk-muted text-caption ml-2">标题、梗概、文体都要填</span>
        <v-spacer />
        <v-btn @click="open = false">取消</v-btn>
        <v-btn color="primary" variant="flat" :disabled="!valid" :loading="saving" @click="submit">建好并打开</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
