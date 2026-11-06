import { argbFromHex, Blend, hexFromArgb, TonalPalette } from '@material/material-color-utilities'
import type { DescKind } from '../api/types'

// 配色只有两个来源：Vuetify 内置默认主题，或用户导入的 material-theme-builder 导出 JSON（SPEC §7）。
// 这里把两者都整理成同一组 Vuetify 颜色键，组件里只用 rgb(var(--v-theme-xxx)) 引用。
// 本文件不包含任何颜色字面值：颜色值只来自运行时读取的默认主题或用户文件。

export type ColorMap = Record<string, string>
export type VarMap = Record<string, string | number>

export interface ThemePair {
  light: { colors: ColorMap; variables: VarMap }
  dark: { colors: ColorMap; variables: VarMap }
}

/** 着色的六种描写类型（叙述、议论不上底色）。 */
export const COLORED_DESC: DescKind[] = ['appearance', 'speech', 'action', 'psychology', 'expression', 'environment']

/** 工作台三层表面：chrome = 活动栏与状态栏，panel = 侧边栏与底部面板，editor = 稿面。 */
const DEFAULT_ALIASES: Record<string, string> = {
  'sk-chrome': 'surface-light',
  'sk-panel': 'surface',
  'sk-editor': 'background',
}

/** 未导入主题时描写类型借用的默认主题角色。Vuetify 默认主题没有 MD3 的 container 角色，只能低透明度借用强调色。 */
const DEFAULT_DESC_SOURCE: Record<string, string> = {
  appearance: 'primary',
  speech: 'secondary',
  action: 'success',
  psychology: 'error',
  expression: 'warning',
  environment: 'info',
}

function aliasColors(base: ColorMap, aliases: Record<string, string>): ColorMap {
  const out: ColorMap = {}
  for (const [key, from] of Object.entries(aliases)) {
    const v = base[from]
    if (v) out[key] = v
  }
  return out
}

/** 在 Vuetify 默认主题之上补齐本应用用到的颜色键。base 取自运行时的默认主题快照。 */
export function defaultThemePair(base: { light: ColorMap; dark: ColorMap }): ThemePair {
  const build = (colors: ColorMap, alpha: number) => {
    const desc: ColorMap = {}
    const variables: VarMap = {}
    for (const k of COLORED_DESC) {
      const v = colors[DEFAULT_DESC_SOURCE[k]!]
      if (v) desc[`desc-${k}`] = v
      variables[`sk-alpha-${k}`] = alpha
    }
    return { colors: { ...colors, ...aliasColors(colors, DEFAULT_ALIASES), ...desc }, variables }
  }
  return { light: build(base.light, 0.16), dark: build(base.dark, 0.3) }
}

// ---------- material-theme-builder 导入 ----------

const HEX = /^#[0-9a-fA-F]{6}$/

/** MTB 角色名（camelCase）→ Vuetify 颜色键。 */
const MTB_ROLE_MAP: Record<string, string> = {
  primary: 'primary',
  onPrimary: 'on-primary',
  primaryContainer: 'primary-container',
  onPrimaryContainer: 'on-primary-container',
  secondary: 'secondary',
  onSecondary: 'on-secondary',
  secondaryContainer: 'secondary-container',
  onSecondaryContainer: 'on-secondary-container',
  tertiary: 'tertiary',
  onTertiary: 'on-tertiary',
  tertiaryContainer: 'tertiary-container',
  onTertiaryContainer: 'on-tertiary-container',
  error: 'error',
  onError: 'on-error',
  errorContainer: 'error-container',
  onErrorContainer: 'on-error-container',
  background: 'background',
  onBackground: 'on-background',
  surface: 'surface',
  onSurface: 'on-surface',
  surfaceVariant: 'surface-variant',
  onSurfaceVariant: 'on-surface-variant',
  outline: 'outline',
  outlineVariant: 'outline-variant',
  inverseSurface: 'inverse-surface',
  inverseOnSurface: 'inverse-on-surface',
  inversePrimary: 'inverse-primary',
  surfaceBright: 'surface-bright',
  surfaceContainerLowest: 'surface-container-lowest',
  surfaceContainerLow: 'surface-container-low',
  surfaceContainer: 'surface-container',
  surfaceContainerHigh: 'surface-container-high',
  surfaceContainerHighest: 'surface-container-highest',
}

const MTB_REQUIRED = ['primary', 'onPrimary', 'secondary', 'error', 'background', 'surface', 'onSurface']

/** SPEC §7 缺少 extended color 时的回退角色与透明度。 */
const MTB_DESC_FALLBACK: Record<string, { role: string; alpha: number }> = {
  appearance: { role: 'primary-container', alpha: 0.85 },
  speech: { role: 'secondary-container', alpha: 0.85 },
  action: { role: 'tertiary-container', alpha: 0.85 },
  psychology: { role: 'error-container', alpha: 0.85 },
  expression: { role: 'surface-variant', alpha: 0.85 },
  environment: { role: 'inverse-primary', alpha: 0.35 },
}

const MTB_ALIASES: Record<string, string> = {
  'sk-chrome': 'surface-container-high',
  'sk-panel': 'surface-container-low',
  'sk-editor': 'surface',
  'surface-light': 'surface-container-high',
  // MTB 没有 warning / info / success 角色。界面里“注意”类提示借用 tertiary，“信息”借用 secondary。
  warning: 'tertiary',
  info: 'secondary',
  success: 'primary',
}

export interface MtbExtendedColor {
  name: string
  color: string
  harmonized?: boolean
}

export type MtbResult = { ok: true; pair: ThemePair; extended: string[] } | { ok: false; error: string }

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function readScheme(raw: unknown, name: string): { colors: ColorMap } | string {
  if (!isRecord(raw)) return `缺少 schemes.${name}`
  const missing = MTB_REQUIRED.filter((k) => typeof raw[k] !== 'string' || !HEX.test(raw[k] as string))
  if (missing.length > 0) return `schemes.${name} 缺少或无法识别：${missing.join('、')}`
  const colors: ColorMap = {}
  for (const [role, key] of Object.entries(MTB_ROLE_MAP)) {
    const v = raw[role]
    if (typeof v === 'string' && HEX.test(v)) colors[key] = v.toUpperCase()
  }
  return { colors }
}

/** extended color 的底色取它自己色调盘里的 container 色调：亮色 90，暗色 30（与 MTB 生成 colorContainer 的规则一致）。 */
function extendedContainer(ext: MtbExtendedColor, sourceHex: string | undefined, dark: boolean): string {
  let argb = argbFromHex(ext.color)
  if (ext.harmonized !== false && sourceHex) argb = Blend.harmonize(argb, argbFromHex(sourceHex))
  return hexFromArgb(TonalPalette.fromInt(argb).tone(dark ? 30 : 90)).toUpperCase()
}

export function themeFromMtb(json: unknown): MtbResult {
  if (!isRecord(json)) return { ok: false, error: '文件不是 JSON 对象' }
  const schemes = json.schemes
  if (!isRecord(schemes)) return { ok: false, error: '缺少 schemes，确认导出的是 material-theme-builder 的 JSON 格式' }
  const light = readScheme(schemes.light, 'light')
  if (typeof light === 'string') return { ok: false, error: light }
  const dark = readScheme(schemes.dark, 'dark')
  if (typeof dark === 'string') return { ok: false, error: dark }

  const core = isRecord(json.coreColors) ? json.coreColors : {}
  const sourceHex =
    typeof json.seed === 'string' && HEX.test(json.seed)
      ? json.seed
      : typeof core.primary === 'string' && HEX.test(core.primary)
        ? core.primary
        : undefined

  const extended = new Map<string, MtbExtendedColor>()
  if (Array.isArray(json.extendedColors)) {
    for (const e of json.extendedColors) {
      if (isRecord(e) && typeof e.name === 'string' && typeof e.color === 'string' && HEX.test(e.color)) {
        extended.set(e.name.trim().toLowerCase(), {
          name: e.name,
          color: e.color,
          harmonized: typeof e.harmonized === 'boolean' ? e.harmonized : true,
        })
      }
    }
  }

  const build = (colors: ColorMap, isDark: boolean) => {
    // 分隔线用 MD3 的 outlineVariant，按原色不透明使用。
    const variables: VarMap = colors['outline-variant']
      ? { 'border-color': colors['outline-variant'], 'border-opacity': 1 }
      : {}
    const out: ColorMap = { ...colors, ...aliasColors(colors, MTB_ALIASES) }
    for (const k of COLORED_DESC) {
      const ext = extended.get(k)
      const fb = MTB_DESC_FALLBACK[k]!
      if (ext) {
        out[`desc-${k}`] = extendedContainer(ext, sourceHex, isDark)
        variables[`sk-alpha-${k}`] = 0.9
      } else if (colors[fb.role]) {
        out[`desc-${k}`] = colors[fb.role]!
        variables[`sk-alpha-${k}`] = fb.alpha
      }
    }
    return { colors: out, variables }
  }

  return {
    ok: true,
    pair: { light: build(light.colors, false), dark: build(dark.colors, true) },
    extended: COLORED_DESC.filter((k) => extended.has(k)),
  }
}
