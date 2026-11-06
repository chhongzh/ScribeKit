import type { DescKind, Genre, MarkKind, ParagraphKind, RelationKind, Rhetoric, WsErrorCode } from '../api/types'

export const GENRES: { value: Genre; title: string }[] = [
  { value: 'narrative', title: '记叙文' },
  { value: 'argumentative', title: '议论文' },
  { value: 'prose', title: '散文' },
  { value: 'poetry', title: '诗歌' },
  { value: 'essay', title: '杂文' },
  { value: 'expository', title: '说明文' },
  { value: 'practical', title: '应用文' },
  { value: 'other', title: '其他' },
]

export function genreLabel(g: string): string {
  return GENRES.find((x) => x.value === g)?.title ?? g
}

export const PARAGRAPH_KINDS: { value: ParagraphKind; title: string; short: string; hint: string }[] = [
  { value: 'normal', title: '普通段', short: '普通', hint: '叙述、描写的主体' },
  { value: 'opening', title: '开头段', short: '开头', hint: '起笔、引出话题或场景' },
  { value: 'closing', title: '结尾段', short: '结尾', hint: '收束、点题' },
  { value: 'lyric', title: '抒情段', short: '抒情', hint: '情感的集中表达' },
  { value: 'transition', title: '转折段', short: '转折', hint: '由你亲自写，Scaffolder 只给建议' },
  { value: 'callback', title: 'Call Back', short: '呼应', hint: '独立段或独句段，展示创意的地方，由你亲自写' },
]

export function kindLabel(k: string): string {
  return PARAGRAPH_KINDS.find((x) => x.value === k)?.title ?? k
}
export function kindShort(k: string): string {
  return PARAGRAPH_KINDS.find((x) => x.value === k)?.short ?? k
}

/** 转折段与 Call Back 段默认由用户自己写，请求时后端降级为建议（SPEC §5 user_owned_paragraph）。 */
export function isUserOwnedKind(k: string): boolean {
  return k === 'transition' || k === 'callback'
}

export const DESC_KINDS: { value: DescKind; title: string; colored: boolean }[] = [
  { value: 'appearance', title: '外貌', colored: true },
  { value: 'speech', title: '语言', colored: true },
  { value: 'action', title: '动作', colored: true },
  { value: 'psychology', title: '心理', colored: true },
  { value: 'expression', title: '神态', colored: true },
  { value: 'environment', title: '环境', colored: true },
  { value: 'narration', title: '叙述', colored: false },
  { value: 'argument', title: '议论', colored: false },
]

export function descLabel(d: string): string {
  return DESC_KINDS.find((x) => x.value === d)?.title ?? d
}

const RHETORIC: Record<Rhetoric, string> = {
  metaphor: '比喻（暗喻）',
  simile: '比喻（明喻）',
  personification: '拟人',
  parallelism: '排比',
  hyperbole: '夸张',
  rhetorical_question: '反问',
  repetition: '反复',
  contrast: '对比',
  synesthesia: '通感',
  quotation: '引用',
}
export function rhetoricLabel(r: string): string {
  return RHETORIC[r as Rhetoric] ?? r
}

const RELATIONS: Record<RelationKind, string> = {
  sequence: '承接',
  cause: '原因',
  result: '结果',
  contrast: '转折',
  concession: '让步',
  progression: '递进',
  parallel: '并列',
  condition: '条件',
  purpose: '目的',
  explanation: '解说',
  supplement: '补充',
  summary: '总结',
}
export function relationLabel(r: string): string {
  return RELATIONS[r as RelationKind] ?? r
}

const MARKS: Record<MarkKind, string> = {
  theme: '点题',
  keyword: '关键词',
  pronoun: '代词',
  callback: '呼应',
  repetition: '重复用词',
  claim: '论点',
  evidence: '论据',
  reasoning: '论证',
}
export function markLabel(m: string): string {
  return MARKS[m as MarkKind] ?? m
}

const WS_ERRORS: Record<WsErrorCode, string> = {
  model_not_configured: '还没有配置模型。在设置里填写 API Key 和模型名后再试。',
  needs_user_input: '先写下标题和梗概，或在光标前写几个字，Scaffolder 才有东西可接。',
  user_owned_paragraph: '这一段由你亲自写：强制生成需要全文至少 300 字的正文作为上下文。',
  cancelled: '已取消。',
  timeout: '模型响应超时，请重试或检查模型服务。',
  bad_request: '请求参数有误，后端拒绝了这次请求。',
  internal: '后端处理时出错。',
}

export function wsErrorText(code: string, message?: string): string {
  const base = WS_ERRORS[code as WsErrorCode]
  if (!base) return message || `未知错误（${code}）`
  if (code === 'bad_request' || code === 'internal') return message ? `${base}${message}` : base
  return base
}
