import { api } from '../api/http'
import type { ArticleSession } from './session'

/** 下载导出文件。打开着的文章先把改动写入后端，否则导出会缺最后几秒的内容。 */
export async function exportArticle(articleId: number, format: 'md' | 'docx', session?: ArticleSession | null) {
  if (session) await session.sync.flush()
  const link = document.createElement('a')
  link.href = api.exportUrl(articleId, format)
  link.download = ''
  document.body.appendChild(link)
  link.click()
  link.remove()
}
