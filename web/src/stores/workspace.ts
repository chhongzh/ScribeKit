import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, describeError } from '../api/http'
import type { Article, ArticleSummary, Collection, Genre } from '../api/types'

/** 资源管理器的数据：文集、每个文集下的文章、散篇。 */
export const useWorkspaceStore = defineStore('workspace', () => {
  const collections = ref<Collection[]>([])
  const articlesByCollection = ref<Record<number, ArticleSummary[]>>({})
  const loose = ref<ArticleSummary[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')

  async function refresh() {
    loading.value = true
    error.value = ''
    try {
      const [cols, looseList] = await Promise.all([api.listCollections(), api.listArticles({ loose: true })])
      const lists = await Promise.all(cols.map((c) => api.listArticles({ collectionId: c.id })))
      const map: Record<number, ArticleSummary[]> = {}
      cols.forEach((c, i) => (map[c.id] = lists[i] ?? []))
      collections.value = cols
      articlesByCollection.value = map
      loose.value = looseList
      loaded.value = true
    } catch (e) {
      error.value = describeError(e)
    } finally {
      loading.value = false
    }
  }

  function findSummary(id: number): ArticleSummary | undefined {
    return (
      loose.value.find((a) => a.id === id) ??
      Object.values(articlesByCollection.value)
        .flat()
        .find((a) => a.id === id)
    )
  }

  async function createCollection(name: string, outline = '') {
    const c = await api.createCollection({ name, outline })
    await refresh()
    return c
  }

  async function renameCollection(id: number, name: string) {
    await api.updateCollection(id, { name })
    await refresh()
  }

  async function deleteCollection(id: number) {
    await api.deleteCollection(id)
    await refresh()
  }

  async function createArticle(b: {
    title: string
    synopsis: string
    genre: Genre
    subgenre: string
    collectionId: number | null
  }): Promise<Article> {
    const a = await api.createArticle({ ...b, collectionId: b.collectionId ?? undefined })
    await refresh()
    return a
  }

  async function renameArticle(id: number, title: string) {
    await api.updateArticle(id, { title })
    await refresh()
  }

  async function deleteArticle(id: number) {
    await api.deleteArticle(id)
    await refresh()
  }

  /** 编辑器里改了标题、文体等元信息后，同步到树上，不必整树刷新。 */
  function patchSummary(id: number, patch: Partial<ArticleSummary>) {
    const s = findSummary(id)
    if (s) Object.assign(s, patch)
  }

  return {
    collections,
    articlesByCollection,
    loose,
    loading,
    loaded,
    error,
    refresh,
    findSummary,
    createCollection,
    renameCollection,
    deleteCollection,
    createArticle,
    renameArticle,
    deleteArticle,
    patchSummary,
  }
})
