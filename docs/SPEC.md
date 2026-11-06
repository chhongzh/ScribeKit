# ScribeKit 实现契约

本文件是前端、后端、语料三条线共同遵守的契约。改动契约必须同时改两端。产品定位与原则见 `FEATURES.md` 之外的需求原文（README 会收录），这里只写工程约定。

## 1. 仓库布局

```
cmd/scribekit/main.go        单二进制入口，启动 Fiber，监听端口，打开浏览器（可关）
internal/config/             配置读取（命令行参数 + 数据目录）
internal/store/              ent schema 与生成代码（internal/store/ent）
internal/textseg/            半句切分算法（与前端 web/src/core/textseg.ts 行为一致）
internal/llm/                模型构建（eino ChatModel，按设置选择 provider）
internal/agent/analyzer/     Analyzer Agent（只读）
internal/agent/scaffolder/   Scaffolder Agent（唯一写入建议的来源）
internal/guide/              写作指引语料（go:embed data/*.md）
internal/api/                REST handler
internal/ws/                 WebSocket hub 与消息分发
internal/export/             Markdown / Docx 导出
web/                         Vite + Vue 3 + Vuetify 3 + TypeScript + Pinia
web/embed.go                 package web，//go:embed all:dist
testdata/corpus/             公有领域范文，用于测试高亮与建议
```

构建：`cd web && bun install && bun run build` 产出 `web/dist`，再 `go build ./cmd/scribekit` 得到单文件。`web/dist/` 被 gitignore，仓库里保留 `web/dist/.gitkeep`，未构建前端时后端仍能编译，访问 `/` 返回一句提示。

运行：`scribekit [-addr 127.0.0.1:7788] [-data <dir>] [-no-browser]`。数据目录默认 `os.UserConfigDir()/ScribeKit`，SQLite 文件 `scribekit.db`。单用户，无鉴权，只监听回环地址。

开发：后端 `go run ./cmd/scribekit -no-browser`，前端 `bun run dev`（Vite 端口 5173，`/api` 含 WebSocket 代理到 7788）。

## 2. 数据模型（ent，SQLite）

ID 一律为 int（ent 默认自增），时间为 RFC3339。JSON 字段名用 camelCase。

| 实体 | 字段 |
|---|---|
| Collection 文集 | id, name, outline(总纲，长文本), createdAt, updatedAt |
| Article 文章 | id, collectionId(可空，空 = 散篇), title, synopsis(梗概), genre, subgenre, createdAt, updatedAt |
| Paragraph 段落 | id, articleId, position(float64，用于任意插入：取前后中值), kind, text, createdAt, updatedAt |
| Material 素材 | id, collectionId(可空), articleId(可空), name, content, createdAt。两者都空 = 全局素材 |
| Suggestion 建议 | id, articleId, requestId, op, target(JSON), paragraphKind, instruction, text, halves(JSON), status, createdAt |
| AnalysisCache | id, paragraphId, textHash, model, result(JSON), createdAt |
| Setting | key(唯一), value(JSON) |

枚举（字符串）：

- `genre`：`narrative`记叙文 `argumentative`议论文 `prose`散文 `poetry`诗歌 `essay`杂文 `expository`说明文 `practical`应用文 `other`。`subgenre` 自由文本（如“应试议论文”“写人记叙文”）。
- `paragraph.kind`：`opening`开头段 `closing`结尾段 `callback`Call Back `lyric`抒情段 `transition`转折段 `normal`普通段。
- `suggestion.op`：`replace`（替换段内区间）`insert_paragraph`（新段落）`advice`（只读建议，不可插入）。
- `suggestion.status`：`streaming` `pending` `applied` `rejected`。

段落文本是唯一的事实来源。句子与半句不入库，由切分算法从文本派生。

## 3. 半句切分算法（两端必须一致）

所有偏移量单位是 **Unicode 码点下标**（Go 用 `[]rune`，TS 用 `Array.from(text)`），区间左闭右开。

1. 句末符号：`。！？!?` 以及连续的 `…`（`……` 视为一个整体）。
2. 半句分隔符：`，；：,;:`。
3. 顿号 `、` 不切分。
4. 分隔符紧跟的闭合符号 `”’」』）)》】"'` 归入前一个片段。
5. 片段包含其结尾标点。片段开头的空白（空格、全角空格、制表符）不计入片段，被跳过。
6. 句子 = 若干连续半句，以句末符号结尾的半句结束一个句子；文本末尾没有句末符号时，剩余部分自成最后一个句子。
7. 全为空白或纯标点的片段丢弃（纯标点片段并入前一个片段的结尾）。
8. 半句 index 在整段内从 0 连续编号，句子 index 同理。

输出结构：

```json
{ "sentences": [ { "index": 0, "start": 0, "end": 12, "halves": [0, 1] } ],
  "halves":    [ { "index": 0, "start": 0, "end": 5, "sentence": 0 } ] }
```

测试向量（两端单测都要覆盖，`|` 表示半句边界）：

- `我走了，他说：“你去吧。”然后` → `我走了，|他说：|“你去吧。”|然后`，句子 `[0,1,2]` `[3]`
- `春天、夏天，都好。` → `春天、夏天，|都好。`
- `啊……他愣住了。` → `啊……|他愣住了。`，两句
- `  空格开头，结尾无标点` → `空格开头，|结尾无标点`，第一个半句 start=2

## 4. REST API（前缀 `/api`，JSON）

错误统一返回 `{ "error": { "code": "...", "message": "..." } }` 与对应 HTTP 状态码。

```
GET    /api/collections                       → Collection[]（带 articleCount）
POST   /api/collections                       {name, outline}
GET    /api/collections/:id                   → Collection & {articles: ArticleSummary[]}
PATCH  /api/collections/:id                   {name?, outline?}
DELETE /api/collections/:id                   文章变为散篇，不删除

GET    /api/articles?collectionId=&loose=1    → ArticleSummary[]（loose=1 只列散篇）
POST   /api/articles                          {title, synopsis, genre, subgenre, collectionId?}
GET    /api/articles/:id                      → Article & {paragraphs: Paragraph[]（按 position 升序）}
PATCH  /api/articles/:id                      {title?, synopsis?, genre?, subgenre?, collectionId?(null = 移出文集)}
DELETE /api/articles/:id

POST   /api/articles/:id/paragraphs           {afterId?: number|null, kind, text}  afterId 省略 = 末尾，null = 开头
PATCH  /api/paragraphs/:id                    {text?, kind?}
POST   /api/paragraphs/:id/move               {afterId: number|null}
DELETE /api/paragraphs/:id
POST   /api/paragraphs/:id/split              {offset}  在码点 offset 处拆成两段，返回 [Paragraph, Paragraph]
POST   /api/paragraphs/:id/merge-next         与下一段合并，返回合并后的 Paragraph

GET    /api/materials?collectionId=&articleId=
POST   /api/materials                          {name, content, collectionId?, articleId?}
POST   /api/materials/upload                   multipart: file(.txt/.md, UTF-8 或 GB18030 自动识别), collectionId?, articleId?
DELETE /api/materials/:id

GET    /api/articles/:id/suggestions?status=pending
PATCH  /api/suggestions/:id                    {halves?: SuggestionHalf[], status?}  前端保存逐半句的评审状态
POST   /api/suggestions/:id/apply              {text}  后端按 op 写入（text 为用户评审后最终文本），返回 {paragraphs: Paragraph[]} 受影响段落
POST   /api/suggestions/:id/reject

GET    /api/guides                             → GuideSummary[] {id, title, genres[], tags[]}
GET    /api/guides/:id                         → {id, title, genres, tags, sources[], body(markdown)}

GET    /api/settings                           → Settings（apiKey 只回显是否已设置）
PUT    /api/settings                           Settings
POST   /api/settings/test                      测试模型连通性 → {ok, message}

GET    /api/articles/:id/export?format=md|docx 下载文件
```

`ArticleSummary` = `{id, collectionId, title, genre, subgenre, updatedAt, charCount}`。

Settings：

```json
{
  "provider": "claude" | "openai",
  "baseUrl": "",
  "apiKey": "",            // PUT 时传空字符串表示不修改
  "apiKeySet": true,       // 只读
  "analyzerModel": "claude-haiku-5-5",
  "scaffolderModel": "claude-sonnet-5-5",
  "autoAnalyze": true,     // 段落停止编辑 1.2s 后自动请求分析
  "theme": null            // material-theme-builder 导出的 JSON 原文，前端写入，后端只存取
}
```

未配置 apiKey 时，WS 的 AI 请求返回 `code: "model_not_configured"`，前端引导去设置页。

## 5. WebSocket（`/api/ws`）

单连接，JSON 文本帧，信封：

```json
{ "id": "客户端生成的请求 id（服务端推送沿用请求 id）", "type": "...", "payload": {} }
```

客户端 → 服务端：

| type | payload |
|---|---|
| `analyze.paragraphs` | `{articleId, paragraphIds: number[]}`，后端按 (paragraphId, textHash) 命中缓存则直接返回 |
| `analyze.article` | `{articleId}` 文章级结构分析 |
| `analyze.text` | `{articleId, paragraphKind, text}` 分析一段尚未落库的文本（建议评审用），结果以 `analyze.textResult` 返回 |
| `scaffold.request` | 见下 |
| `cancel` | `{targetId}` 取消某个请求 |
| `ping` | `{}` |

`scaffold.request`：

```json
{
  "articleId": 1,
  "cursor": { "paragraphId": 3, "offset": 17 } | null,
  "selection": { "paragraphId": 3, "start": 10, "end": 17 } | null,
  "instruction": "补一段环境描写",       // 可空。前端从段落中的 [ ... ] 命令或命令面板输入得到
  "commandRange": { "paragraphId": 3, "start": 20, "end": 29 } | null, // [ ... ] 命令自身所在区间，结果默认替换它
  "mode": "complete" | "generate" | "polish" | "detail" | "advise",
  "newParagraphKind": "lyric" | null,     // 非空 = 结果作为新段落插在 cursor 段之后
  "force": false                          // 对 transition/callback 段强制生成
}
```

`mode` 语义：`complete` 句子补全（续写光标所在的未完成半句/句子），`generate` 按指令生成，`polish` 润色选区（只改句式不改意思），`detail` 为选区补充细节，`advise` 只给建议。

服务端 → 客户端：

| type | payload |
|---|---|
| `analyze.result` | `ParagraphAnalysis`（见 §6） |
| `analyze.articleResult` | `ArticleAnalysis` |
| `analyze.textResult` | `ParagraphAnalysis`，paragraphId = 0 |
| `scaffold.started` | `{suggestionId}` |
| `scaffold.delta` | `{suggestionId, text}` 增量文本 |
| `scaffold.done` | `Suggestion`（完整，status = pending） |
| `error` | `{code, message}`，id 为出错请求的 id |
| `pong` | `{}` |

错误码：`model_not_configured` `needs_user_input`（标题或梗概为空，或光标处没有用户文字可续写）`user_owned_paragraph`（转折段 / Call Back 段且未 force，此时 Scaffolder 自动降级为 advise，不报错；只有 force 但用户正文不足 300 字时才报此错）`cancelled` `bad_request` `internal`。

`Suggestion`：

```json
{
  "id": 9, "articleId": 1, "requestId": "r-12", "op": "replace",
  "target": { "paragraphId": 3, "start": 20, "end": 29 }    // replace
          | { "afterParagraphId": 3 }                       // insert_paragraph，null = 开头
          | null,                                           // advice
  "paragraphKind": "normal",
  "instruction": "补一段环境描写",
  "text": "完整生成文本",
  "halves": [ { "index": 0, "start": 0, "end": 8, "text": "...", "state": "pending" | "accepted" | "rejected" | "edited", "edited": "用户改写后的文本，可空" } ],
  "rationale": "一两句说明为什么这样写（advice 时为主体内容）",
  "status": "pending",
  "createdAt": "..."
}
```

`halves` 由后端用 §3 算法对 `text` 切分得到。前端评审完毕后，最终文本 = 按顺序拼接 `accepted` 的原文与 `edited` 的改写，`rejected` 丢弃，然后调用 `/apply`。生成完成后前端自动发 `analyze.text` 让用户看到高亮后再决定。

## 6. Analyzer 输出

```ts
type DescKind = 'appearance' | 'speech' | 'action' | 'psychology' | 'expression' | 'environment' | 'narration' | 'argument'
// 外貌 语言 动作 心理 神态 环境 叙述 议论；一个半句可以有多个

type Rhetoric = 'metaphor' | 'simile' | 'personification' | 'parallelism' | 'hyperbole' | 'rhetorical_question'
              | 'repetition' | 'contrast' | 'synesthesia' | 'quotation'

type RelationKind = 'sequence' | 'cause' | 'result' | 'contrast' | 'concession' | 'progression' | 'parallel'
                  | 'condition' | 'purpose' | 'explanation' | 'supplement' | 'summary'

type MarkKind = 'theme' | 'keyword' | 'pronoun' | 'callback' | 'repetition' | 'claim' | 'evidence' | 'reasoning'
// 点题字眼 关键词 代词（及其指代） 呼应 重复用词 论点 论据 论证

interface ParagraphAnalysis {
  paragraphId: number
  textHash: string            // sha256(text) 前 16 位十六进制，前端用来丢弃过期结果
  segmentation: Segmentation  // §3 的结构
  halves: { index: number; desc: DescKind[]; rhetoric: Rhetoric[] }[]
  relations: { from: number; to: number; kind: RelationKind; note?: string }[]  // 半句 index
  marks: { start: number; end: number; kind: MarkKind; note?: string; ref?: { start: number; end: number } }[]
  diagnostics: { start: number; end: number; severity: 'hint' | 'info' | 'warning'; message: string }[]
  model: string
}

interface ArticleAnalysis {
  articleId: number
  themeWords: string[]                         // 点题字眼，供全篇 marks 对齐
  themeHits: { paragraphId: number; start: number; end: number }[]  // 实际点题位置
  paragraphs: { paragraphId: number; detectedKind: ParagraphKind; role: string; note?: string }[]
  structure: { name: string; fit: string; gaps: string[] }  // 例如套用“议论文 引议联结”时的对照，不打分
  notes: string[]
}
```

Analyzer 原则：只读不写，不生成任何可插入文本，不打分。`diagnostics` 只陈述事实（“与第 2 段重复使用‘仿佛’”“本段没有动作描写，人物停在原地”），不给改写结果。

前端渲染约定：
- 描写类型 → 半句底色（取主题 extended color，见 §7）。
- `theme` → 方框；`keyword` → 实线下划线；`repetition` → 波浪下划线；`pronoun` → 虚线下划线，悬停显示指代并高亮 `ref`；`callback` → 双下划线；`claim/evidence/reasoning` → 左侧装订线标记。
- `relations` → 半句之间的箭头（悬停或开启“逻辑”图层时绘制 SVG 弧线），标注关系名。
- `diagnostics` → 类似 LSP 的点状下划线 + Problems 面板。

## 7. 配色

前端禁止手写颜色值。主题来自 material-theme-builder 导出的 JSON（`schemes.light` / `schemes.dark` + `extendedColors`），在设置页导入，持久化到 Settings.theme。未导入时使用 Vuetify 内置默认主题。

描写类型的颜色用 MTB 的 extended colors，名称约定：`appearance` `speech` `action` `psychology` `expression` `environment`。导入的 JSON 缺少某个 extended color 时，按以下 MD3 角色回退：appearance→primaryContainer，speech→secondaryContainer，action→tertiaryContainer，psychology→errorContainer，expression→surfaceVariant，environment→inversePrimary（低透明度）。

## 8. 写作指引语料（internal/guide/data/*.md）

每个文件一条指引，YAML front matter：

```yaml
---
id: argumentative-structure
title: 议论文结构：论点、论据、论证
genres: [argumentative]
tags: [structure, exam]
sources:
  - title: 来源标题
    url: https://...
---
正文 markdown。分三节：## 要点（给人看）## 用于 Scaffolder（给模型的约束条目，条目化）## 用于 Analyzer（识别信号，条目化）
```

Scaffolder 按文章 genre 加载匹配的指引的“用于 Scaffolder”节，Analyzer 加载“用于 Analyzer”节。语料只写转述和归纳，不整段复制受版权保护的原文。

## 9. Agent 调度（eino ADK）

- `internal/llm` 根据 Settings 构建 `model.ToolCallingChatModel`：`claude` 走 eino-ext claude 组件，`openai` 走 eino-ext openai 组件（兼容任意 OpenAI 协议服务，baseUrl 可配）。设置变更后重建。
- Analyzer：`adk.ChatModelAgent`，无工具，要求严格 JSON 输出，后端解析失败时重试一次；按 (paragraphId, textHash, model) 缓存。并发上限 3。
- Scaffolder：`adk.ChatModelAgent`，工具：`read_article`（当前文章全部段落）、`list_collection`（同文集其他文章的标题、梗概、开头片段）、`read_article_by_id`、`search_materials`（关键词 bigram 重叠排序，返回片段）、`read_outline`、`read_guide`。流式输出最终文本，通过 `scaffold.delta` 推送。最终回答格式：正文与 `<rationale>...</rationale>` 分开，后端拆分。
- 每个 WS 请求绑定 context，`cancel` 或连接断开时取消。
- 生成前置检查：标题和梗概都非空，否则 `needs_user_input`。`complete` 模式要求光标前有用户文字。

## 10. 提示词工程（internal/prompt）

所有发给模型的提示词（系统提示、用户消息模板、工具描述、JSON 修复重试提示）集中在 `internal/prompt` 包，其他包不得内联提示词字符串。

- 模板文件：`internal/prompt/templates/<agent>/<name>.jinja`，公共片段放 `templates/_partials/`（产品原则、禁用套话清单、JSON 输出格式说明、段落类型说明），`go:embed` 加载，Jinja2 语法。
- 渲染：eino 的 `schema.Jinja2` 底层是 gonja，但它逐条 message 用无 loader 的字符串渲染，`{% include %}` / `{% import %}` 不可用。因此本包直接用 gonja（与 eino 同版本依赖）配一个基于 embed FS 的 loader 渲染，产出 `[]*schema.Message` 交给 eino agent。
- 动态展开：模板只接收结构化变量（文章元信息、段落列表、带编号的半句、指引小节、素材片段、总纲、mode、paragraphKind、genre 等），条件、循环、可选块都写在模板里，Go 代码里不拼接提示词文本。
- 每个模板对应一个强类型变量结构体和渲染函数，例如 `prompt.AnalyzerParagraph(vars AnalyzerParagraphVars) ([]*schema.Message, error)`。包初始化时预解析全部模板，语法错误在启动与单测时立即暴露。
- 单测：每个模板用固定变量渲染并与 golden 文件比对（`internal/prompt/testdata/*.golden`，`-update` 标志重写），覆盖各 mode、paragraphKind、genre 分支。

## 11. 提交规范

按功能提交，提交信息中文或英文均可，不得包含任何 Co-Authored-By 或合作者署名行。
