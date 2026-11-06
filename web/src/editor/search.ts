import type { Node as PMNode } from 'prosemirror-model'

export interface SearchMatch {
  from: number
  to: number
  replacement: string
}

export function searchDocument(doc: PMNode, query: string, replacement: string, matchCase: boolean, regex: boolean): { matches: SearchMatch[]; error: string } {
  if (!query) return { matches: [], error: '' }
  let pattern: RegExp
  try {
    pattern = new RegExp(regex ? query : query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), matchCase ? 'gu' : 'giu')
  } catch {
    return { matches: [], error: '正则表达式无效' }
  }
  const matches: SearchMatch[] = []
  doc.forEach((paragraph, offset) => {
    const text = paragraph.textBetween(0, paragraph.content.size, '', '\n')
    for (const match of text.matchAll(pattern)) {
      if (!match[0].length) continue
      const from = offset + 1 + match.index
      matches.push({ from, to: from + match[0].length, replacement: regex ? expandReplacement(replacement, match, text) : replacement })
    }
  })
  return { matches, error: '' }
}

function expandReplacement(replacement: string, match: RegExpMatchArray, text: string): string {
  return replacement.replace(/\$(\$|&|`|'|\d{1,2}|<[^>]+>)/g, (token, group: string) => {
    if (group === '$') return '$'
    if (group === '&') return match[0]
    if (group === '`') return text.slice(0, match.index)
    if (group === "'") return text.slice(match.index! + match[0].length)
    if (group.startsWith('<')) return match.groups?.[group.slice(1, -1)] ?? token
    const index = Number(group)
    if (index > 0 && index < match.length) return match[index] ?? ''
    if (group.length === 2 && Number(group[0]) > 0 && Number(group[0]) < match.length) return `${match[Number(group[0])] ?? ''}${group[1]}`
    return token
  })
}
