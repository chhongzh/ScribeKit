# ScribeKit

一个写作文用的 IDE。用户逐段、逐句、逐半句做决定，AI 只做描写生成、句式润色、细节补充这些苦力活，所有 AI 输出都是要逐半句评审的建议。

- 需求原文：[docs/REQUIREMENTS.md](docs/REQUIREMENTS.md)
- 前后端契约：[docs/SPEC.md](docs/SPEC.md)

## 运行

开发模式（两个终端）：

```bash
go run ./cmd/scribekit -no-browser
```

```bash
cd web && bun install && bun run dev
```

然后打开 http://localhost:5173（`/api` 与 WebSocket 由 Vite 代理到 7788）。

单文件构建：

```bash
cd web && bun install && bun run build && cd .. && go build ./cmd/scribekit
```

运行 `scribekit [-addr 127.0.0.1:7788] [-data <目录>] [-no-browser] [-jsonlog]`，数据默认存在系统配置目录下的 `ScribeKit/scribekit.db`。

## 发布

GitHub Actions 在 PR、推送到 `main` / `master` 和推送 `v*` 标签时运行前后端测试，并通过 GoReleaser 构建包含前端的单文件程序。Windows、Linux、macOS 均提供 amd64 和 arm64 版本，Windows 使用 ZIP，其他平台使用 tar.gz，并生成 `checksums.txt`。

推送版本标签（例如 `v0.1.3`）后，测试和快照构建通过才会发布 GitHub Release；带预发布版本号的标签（例如 `v0.1.3-rc.1`）会标记为预发布。发布使用仓库自带的 `GITHUB_TOKEN`。

本地安装 Go 1.27.3、Node.js 24、Bun 1.3.14 和 GoReleaser v2.20.0 后，在仓库根目录执行 `goreleaser release --snapshot --clean` 可验证完整打包流程，产物位于 `dist/`。

## 配置模型

在设置页填写 provider（`claude` 或兼容 OpenAI 协议的 `openai` + baseUrl）、API key、Analyzer 与 Scaffolder 的模型名，点“测试连通性”。没有配置时，所有 AI 请求会提示先去设置。

配色：在设置页导入 material-theme-builder 导出的 JSON。描写类型的底色取 extended colors 中名为 `appearance` `speech` `action` `psychology` `expression` `environment` 的颜色。

## AI 日志

后端统一通过 Zap 向标准错误输出日志，默认使用彩色控制台格式，以不同颜色区分日志级别；添加 `--jsonlog` 参数切换为无颜色的 JSON 日志，例如 `go run ./cmd/scribekit -no-browser --jsonlog`。两种格式都不采样。覆盖启动与退出、API 错误、WebSocket AI 请求、模型配置与构建缓存、连接测试、提示词渲染、Analyzer 并发等待与分析缓存、JSON 修复重试、Scaffolder 规划与建议保存/清理，以及 Eino 组件和工具调用。每次操作记录开始、结束、耗时、失败、取消或超时；流式响应记录片段数和输出字节数，模型提供用量时记录 Token 数。

可用 `request_id` 关联一次 WebSocket 请求，`span_id` 区分具体操作。日志不输出 API Key、模型地址、完整提示词、文章正文或工具参数/结果正文；错误字段保留底层错误信息。提示词渲染日志只有模板名、耗时和输出字节数。

逐半句分析中，`submit_half` 参数截断、JSON 格式错误或字段类型错误会作为工具反馈交给模型修正，已完成的半句继续保留；不完整参数不会生成部分分析结果。修正仍受原有 90 秒超时和迭代上限限制，只有覆盖全部半句的结果才会写入缓存。正常完成工具收集的内部结束信号记为 `ai.completed`，真实失败、取消和超时继续分别记录。

Analyzer 单次分析（含排队和 JSON 修复重试）最多 90 秒，WebSocket AI 请求最多 3 分钟；前端在 190 秒未收到结束结果时释放状态并取消请求。点击状态栏“后台 Agent：n 个”打开“后台任务”，统一查看段落分析、全文分析、类型推断、建议文本分析和写作生成的状态，并单独取消任务。任务分为准备中、运行中、已完成、已取消和失败，失败显示原因；保留最近 100 条已结束记录，支持清除，运行中的任务不会被清除。Agent 数来自服务端实际运行数量，任务数包含准备中的请求，一个任务可能调用多个 Agent。任务记录仅保留在当前页面，断线时终止在途请求。重复提交相同段落和文本会合并，关闭文章会取消它的后台分析和生成。保存请求超过 30 秒会提示失败。

界面使用 Vuetify 的 MD3 blueprint。正文默认以 100% 显示，可在状态栏调整到 75%–150%；缩放只改变稿面字号。

侧栏与底部面板展开/收起、视图与标签切换、菜单和对话框、主题色及悬停状态使用统一的过渡时长与缓动，让状态变化更容易跟上。拖动分隔条时尺寸直接跟手；系统开启“减少动态效果”后关闭过渡。所有颜色继续来自 Vuetify 主题变量。

右键编辑器标签页可选择“关闭当前”“关闭右侧”“关闭全部”“关闭其他”，操作针对右键选中的标签页。也可聚焦标签页后按 Shift+F10 或菜单键，或点击标签栏右侧的“标签页操作”按钮。关闭前会先保存文章，保存失败时询问是否丢弃，取消会停止本次批量关闭；关闭标签页不会删除文章。

## 编辑器快捷键

| 按键 | 作用 |
|---|---|
| Enter | 放下当前段，新建普通段 |
| Alt+Enter | 指定类型新建段落 |
| Alt+T | 本段类型菜单（含上移、下移、删除） |
| Alt+Shift+↑/↓ | 移动段落（也可拖动装订线） |
| Shift+Enter | 交给 Scaffolder：有 `[指令]` 按指令生成，光标前有未写完的半句则续写，否则打开命令面板 |
| Ctrl+K / Ctrl+Space | 光标处命令面板 |
| Ctrl+F / Ctrl+H | 查找 / 查找并替换；支持区分大小写与正则，全部替换可一次撤销 |
| F11 | 全屏编辑 |
| Ctrl+Enter | 段内换行（诗歌分行） |
| Esc | 取消正在生成的建议 |
| A / R / E | 评审视图中接受 / 拒绝 / 改写当前半句 |

命令面板还提供“AI 重新整理全文段落类型”：在后台生成每段类型和理由，期间可继续写作或切换文章，手动切换该文章的段落类型暂时锁定。关闭进度窗口会继续运行，只有点击“取消推断”或任务列表中的“取消任务”才会停止并解锁；完成、失败、断线和超时也会释放锁。完成后从任务列表的“查看结果”或完成通知打开预览，确认后一次应用，Ctrl+Z 可撤销。正文或段落类型已变化时，旧结果不能应用，需要重新推断。写作命令点击后才展开配置。底部面板收起后可从状态栏的“问题”“输出”或后台 Agent 按钮重新打开。状态栏每 500 毫秒接收后台实际运行的 Agent 数和服务商返回的 token 用量，累计范围为本次服务启动以来；服务商未返回用量时会明确提示，绝不按文本估算。

梗概、总纲、素材内容等多行输入框右上角均可进入全屏编辑，按 Esc 或点击退出按钮返回，保留输入内容和光标位置。

## 许可

见 [LICENSE](LICENSE)。
