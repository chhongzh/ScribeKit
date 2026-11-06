// 与 docs/SPEC.md §2 §4 §5 §6 一一对应。改这里之前先改 SPEC。

import type { Segmentation } from '../core/textseg'

export type Genre = 'narrative' | 'argumentative' | 'prose' | 'poetry' | 'essay' | 'expository' | 'practical' | 'other'
export type ParagraphKind = 'opening' | 'closing' | 'callback' | 'lyric' | 'transition' | 'normal'
export type SuggestionOp = 'replace' | 'insert_paragraph' | 'advice'
export type SuggestionStatus = 'streaming' | 'pending' | 'applied' | 'rejected'

export interface Collection {
  id: number
  name: string
  outline: string
  articleCount?: number
  createdAt: string
  updatedAt: string
}

export interface ArticleSummary {
  id: number
  collectionId: number | null
  title: string
  genre: Genre
  subgenre: string
  updatedAt: string
  charCount: number
}

export interface CollectionDetail extends Collection {
  articles: ArticleSummary[]
}

export interface Paragraph {
  id: number
  articleId: number
  position: number
  kind: ParagraphKind
  text: string
  createdAt: string
  updatedAt: string
}

export interface Article {
  id: number
  collectionId: number | null
  title: string
  synopsis: string
  genre: Genre
  subgenre: string
  createdAt: string
  updatedAt: string
}

export interface ArticleDetail extends Article {
  paragraphs: Paragraph[]
}

export interface Material {
  id: number
  collectionId: number | null
  articleId: number | null
  name: string
  content: string
  createdAt: string
}

export type HalfState = 'pending' | 'accepted' | 'rejected' | 'edited'

export interface SuggestionHalf {
  index: number
  start: number
  end: number
  text: string
  state: HalfState
  edited?: string | null
}

export type SuggestionTarget =
  | { paragraphId: number; start: number; end: number }
  | { afterParagraphId: number | null }
  | null

export interface Suggestion {
  id: number
  articleId: number
  requestId: string
  op: SuggestionOp
  target: SuggestionTarget
  paragraphKind: ParagraphKind
  instruction: string
  text: string
  halves: SuggestionHalf[]
  rationale?: string
  status: SuggestionStatus
  createdAt: string
}

export interface GuideSummary {
  id: string
  title: string
  genres: string[]
  tags: string[]
}

export interface GuideDetail extends GuideSummary {
  sources: { title: string; url: string }[]
  body: string
}

export interface Settings {
  provider: 'claude' | 'openai'
  baseUrl: string
  apiKey: string
  apiKeySet: boolean
  analyzerModel: string
  scaffolderModel: string
  autoAnalyze: boolean
  theme: unknown
}

export type DescKind =
  | 'appearance'
  | 'speech'
  | 'action'
  | 'psychology'
  | 'expression'
  | 'environment'
  | 'narration'
  | 'argument'

export type Rhetoric =
  | 'metaphor'
  | 'simile'
  | 'personification'
  | 'parallelism'
  | 'hyperbole'
  | 'rhetorical_question'
  | 'repetition'
  | 'contrast'
  | 'synesthesia'
  | 'quotation'

export type RelationKind =
  | 'sequence'
  | 'cause'
  | 'result'
  | 'contrast'
  | 'concession'
  | 'progression'
  | 'parallel'
  | 'condition'
  | 'purpose'
  | 'explanation'
  | 'supplement'
  | 'summary'

export type MarkKind = 'theme' | 'keyword' | 'pronoun' | 'callback' | 'repetition' | 'claim' | 'evidence' | 'reasoning'

export interface AnalysisMark {
  start: number
  end: number
  kind: MarkKind
  note?: string
  ref?: { start: number; end: number }
}

export interface Diagnostic {
  start: number
  end: number
  severity: 'hint' | 'info' | 'warning'
  message: string
}

export interface ParagraphAnalysis {
  paragraphId: number
  textHash: string
  segmentation: Segmentation
  halves: { index: number; desc: DescKind[]; rhetoric: Rhetoric[] }[]
  relations: { from: number; to: number; kind: RelationKind; note?: string }[]
  marks: AnalysisMark[]
  diagnostics: Diagnostic[]
  model: string
}

export interface ArticleAnalysis {
  articleId: number
  themeWords: string[]
  themeHits: { paragraphId: number; start: number; end: number }[]
  paragraphs: { paragraphId: number; detectedKind: ParagraphKind; role: string; note?: string }[]
  structure: { name: string; fit: string; gaps: string[] }
  notes: string[]
}

export type ScaffoldMode = 'complete' | 'generate' | 'polish' | 'detail' | 'advise'

export interface ScaffoldRequest {
  articleId: number
  cursor: { paragraphId: number; offset: number } | null
  selection: { paragraphId: number; start: number; end: number } | null
  instruction: string
  commandRange: { paragraphId: number; start: number; end: number } | null
  mode: ScaffoldMode
  newParagraphKind: ParagraphKind | null
  force: boolean
}

export type WsErrorCode =
  | 'model_not_configured'
  | 'needs_user_input'
  | 'user_owned_paragraph'
  | 'cancelled'
  | 'bad_request'
  | 'internal'

export interface ApiErrorBody {
  error: { code: string; message: string }
}
