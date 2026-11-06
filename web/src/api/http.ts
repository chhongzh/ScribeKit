import type {
  Article,
  ArticleDetail,
  ArticleSummary,
  Collection,
  CollectionDetail,
  Genre,
  GuideDetail,
  GuideSummary,
  Material,
  Paragraph,
  ParagraphKind,
  Settings,
  Suggestion,
  SuggestionHalf,
  SuggestionStatus,
} from './types'

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message)
  }
}

/** 给人看的错误说明：网络不通与后端拒绝分开讲。 */
export function describeError(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 0) return '连不上后端（127.0.0.1:7788）。确认 scribekit 正在运行。'
    if (e.status === 404) return `找不到对象，可能已被删除。${e.message ? `（${e.message}）` : ''}`
    return e.message || `后端返回 ${e.status}`
  }
  if (e instanceof Error) return e.message
  return String(e)
}

async function request<T>(method: string, path: string, body?: unknown, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(`/api${path}`, {
      method,
      headers: body !== undefined && !(body instanceof FormData) ? { 'Content-Type': 'application/json' } : undefined,
      body: body === undefined ? undefined : body instanceof FormData ? body : JSON.stringify(body),
      ...init,
    })
  } catch (e) {
    throw new ApiError(0, 'network', e instanceof Error ? e.message : String(e))
  }
  if (!res.ok) {
    let code = 'http_' + res.status
    let message = ''
    try {
      const j = (await res.json()) as { error?: { code?: string; message?: string } }
      code = j.error?.code ?? code
      message = j.error?.message ?? ''
    } catch {
      message = res.statusText
    }
    throw new ApiError(res.status, code, message)
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return (text ? JSON.parse(text) : undefined) as T
}

function qs(params: Record<string, string | number | null | undefined>): string {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') p.set(k, String(v))
  const s = p.toString()
  return s ? `?${s}` : ''
}

export const api = {
  listCollections: () => request<Collection[]>('GET', '/collections'),
  getCollection: (id: number) => request<CollectionDetail>('GET', `/collections/${id}`),
  createCollection: (b: { name: string; outline: string }) => request<Collection>('POST', '/collections', b),
  updateCollection: (id: number, b: { name?: string; outline?: string }) =>
    request<Collection>('PATCH', `/collections/${id}`, b),
  deleteCollection: (id: number) => request<void>('DELETE', `/collections/${id}`),

  listArticles: (q: { collectionId?: number; loose?: boolean }) =>
    request<ArticleSummary[]>('GET', `/articles${qs({ collectionId: q.collectionId, loose: q.loose ? 1 : undefined })}`),
  createArticle: (b: { title: string; synopsis: string; genre: Genre; subgenre: string; collectionId?: number | null }) =>
    request<Article>('POST', '/articles', b),
  getArticle: (id: number) => request<ArticleDetail>('GET', `/articles/${id}`),
  updateArticle: (
    id: number,
    b: { title?: string; synopsis?: string; genre?: Genre; subgenre?: string; collectionId?: number | null },
  ) => request<Article>('PATCH', `/articles/${id}`, b),
  deleteArticle: (id: number) => request<void>('DELETE', `/articles/${id}`),

  createParagraph: (articleId: number, b: { afterId?: number | null; kind: ParagraphKind; text: string }) =>
    request<Paragraph>('POST', `/articles/${articleId}/paragraphs`, b),
  updateParagraph: (id: number, b: { text?: string; kind?: ParagraphKind }) =>
    request<Paragraph>('PATCH', `/paragraphs/${id}`, b),
  moveParagraph: (id: number, afterId: number | null) => request<Paragraph>('POST', `/paragraphs/${id}/move`, { afterId }),
  deleteParagraph: (id: number) => request<void>('DELETE', `/paragraphs/${id}`),
  splitParagraph: (id: number, offset: number) =>
    request<[Paragraph, Paragraph]>('POST', `/paragraphs/${id}/split`, { offset }),
  mergeNext: (id: number) => request<Paragraph>('POST', `/paragraphs/${id}/merge-next`),

  listMaterials: (q: { collectionId?: number; articleId?: number }) =>
    request<Material[]>('GET', `/materials${qs(q)}`),
  createMaterial: (b: { name: string; content: string; collectionId?: number | null; articleId?: number | null }) =>
    request<Material>('POST', '/materials', b),
  uploadMaterial: (file: File, owner: { collectionId?: number | null; articleId?: number | null }) => {
    const fd = new FormData()
    fd.append('file', file)
    if (owner.collectionId) fd.append('collectionId', String(owner.collectionId))
    if (owner.articleId) fd.append('articleId', String(owner.articleId))
    return request<Material>('POST', '/materials/upload', fd)
  },
  deleteMaterial: (id: number) => request<void>('DELETE', `/materials/${id}`),

  listSuggestions: (articleId: number, status: SuggestionStatus) =>
    request<Suggestion[]>('GET', `/articles/${articleId}/suggestions${qs({ status })}`),
  updateSuggestion: (id: number, b: { halves?: SuggestionHalf[]; status?: SuggestionStatus }) =>
    request<Suggestion>('PATCH', `/suggestions/${id}`, b),
  applySuggestion: (id: number, text: string) =>
    request<{ paragraphs: Paragraph[] }>('POST', `/suggestions/${id}/apply`, { text }),
  rejectSuggestion: (id: number) => request<unknown>('POST', `/suggestions/${id}/reject`),

  listGuides: () => request<GuideSummary[]>('GET', '/guides'),
  getGuide: (id: string) => request<GuideDetail>('GET', `/guides/${encodeURIComponent(id)}`),

  getSettings: () => request<Settings>('GET', '/settings'),
  putSettings: (s: Settings) => request<Settings>('PUT', '/settings', s),
  testSettings: () => request<{ ok: boolean; message: string }>('POST', '/settings/test'),

  exportUrl: (articleId: number, format: 'md' | 'docx') => `/api/articles/${articleId}/export?format=${format}`,
}
