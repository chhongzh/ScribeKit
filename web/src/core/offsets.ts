// SPEC 偏移是 Unicode 码点下标，ProseMirror 与 JS 字符串是 UTF-16 下标。
// 两者只在 BMP 之外的字符（扩展区汉字、emoji）上不同，所有换算都必须经过这里。

function isHigh(code: number): boolean {
  return code >= 0xd800 && code <= 0xdbff
}
function isLow(code: number): boolean {
  return code >= 0xdc00 && code <= 0xdfff
}

function stepAt(text: string, u: number): number {
  return isHigh(text.charCodeAt(u)) && u + 1 < text.length && isLow(text.charCodeAt(u + 1)) ? 2 : 1
}

/** 码点下标 → UTF-16 下标。超出范围时夹到文本末尾。 */
export function cpToU16(text: string, cp: number): number {
  if (cp <= 0) return 0
  let u = 0
  let c = 0
  while (u < text.length && c < cp) {
    u += stepAt(text, u)
    c++
  }
  return u
}

/** UTF-16 下标 → 码点下标。落在代理对中间时向前取整。 */
export function u16ToCp(text: string, u16: number): number {
  const limit = Math.min(Math.max(u16, 0), text.length)
  let u = 0
  let c = 0
  while (u < limit) {
    const step = stepAt(text, u)
    if (u + step > limit) break
    u += step
    c++
  }
  return c
}

export function cpLength(text: string): number {
  return u16ToCp(text, text.length)
}

/** 按码点区间截取。 */
export function sliceCp(text: string, start: number, end: number): string {
  return text.slice(cpToU16(text, start), cpToU16(text, end))
}

/** 字数：不含空白的码点数。 */
export function countChars(text: string): number {
  let n = 0
  for (const ch of text) if (!/\s/u.test(ch)) n++
  return n
}
