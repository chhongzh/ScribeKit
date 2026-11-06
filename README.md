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

运行 `scribekit [-addr 127.0.0.1:7788] [-data <目录>] [-no-browser]`，数据默认存在系统配置目录下的 `ScribeKit/scribekit.db`。

## 配置模型

在设置页填写 provider（`claude` 或兼容 OpenAI 协议的 `openai` + baseUrl）、API key、Analyzer 与 Scaffolder 的模型名，点“测试连通性”。没有配置时，所有 AI 请求会提示先去设置。

配色：在设置页导入 material-theme-builder 导出的 JSON。描写类型的底色取 extended colors 中名为 `appearance` `speech` `action` `psychology` `expression` `environment` 的颜色。

## 编辑器快捷键

| 按键 | 作用 |
|---|---|
| Enter | 放下当前段，新建普通段 |
| Alt+Enter | 指定类型新建段落 |
| Alt+T | 本段类型菜单（含上移、下移、删除） |
| Alt+Shift+↑/↓ | 移动段落（也可拖动装订线） |
| Shift+Enter | 交给 Scaffolder：有 `[指令]` 按指令生成，光标前有未写完的半句则续写，否则打开命令面板 |
| Ctrl+K / Ctrl+Space | 光标处命令面板 |
| Ctrl+Enter | 段内换行（诗歌分行） |
| Esc | 取消正在生成的建议 |
| A / R / E | 评审视图中接受 / 拒绝 / 改写当前半句 |

## 许可

见 [LICENSE](LICENSE)。
