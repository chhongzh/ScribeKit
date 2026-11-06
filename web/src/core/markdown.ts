import DOMPurify from 'dompurify'
import { marked } from 'marked'

// 指引正文来自后端语料，渲染前一律净化：去掉脚本、事件属性、内联样式；外链在新窗口打开。

DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A') {
    node.setAttribute('target', '_blank')
    node.setAttribute('rel', 'noopener noreferrer')
  }
})

export function renderMarkdown(src: string): string {
  const html = marked.parse(src, { async: false, gfm: true, breaks: false }) as string
  return DOMPurify.sanitize(html, { FORBID_ATTR: ['style'], FORBID_TAGS: ['style', 'iframe', 'form'] })
}
