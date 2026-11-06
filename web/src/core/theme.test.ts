import { describe, expect, test } from 'bun:test'
import sample from './__fixtures__/mtb-sample.json'
import { COLORED_DESC, themeFromMtb } from './theme'

// mtb-sample.json 由 material-color-utilities 按 MTB 的规则从种子色生成，代表用户导入的文件。

describe('material-theme-builder 导入', () => {
  test('读取亮暗两套 scheme 并映射到 Vuetify 键', () => {
    const r = themeFromMtb(sample)
    expect(r.ok).toBe(true)
    if (!r.ok) return
    expect(r.pair.light.colors.primary).toBe(sample.schemes.light.primary.toUpperCase())
    expect(r.pair.dark.colors['on-surface']).toBe(sample.schemes.dark.onSurface.toUpperCase())
    expect(r.pair.light.colors['sk-editor']).toBe(sample.schemes.light.surface.toUpperCase())
  })

  test('extended color 按名称匹配（大小写无关），缺失的按 SPEC §7 回退', () => {
    const r = themeFromMtb(sample)
    if (!r.ok) throw new Error(r.error)
    expect(r.extended.sort()).toEqual(['appearance', 'speech'])
    expect(r.pair.light.colors['desc-action']).toBe(sample.schemes.light.tertiaryContainer.toUpperCase())
    expect(r.pair.dark.colors['desc-environment']).toBe(sample.schemes.dark.inversePrimary.toUpperCase())
    expect(r.pair.dark.variables['sk-alpha-environment']).toBeLessThan(0.5)
    for (const k of COLORED_DESC) {
      expect(r.pair.light.colors[`desc-${k}`]).toMatch(/^#[0-9A-F]{6}$/)
      expect(r.pair.dark.colors[`desc-${k}`]).toMatch(/^#[0-9A-F]{6}$/)
    }
    expect(r.pair.light.colors['desc-appearance']).not.toBe(r.pair.dark.colors['desc-appearance'])
  })

  test('拒绝不完整的文件并说明缺什么', () => {
    expect(themeFromMtb([])).toEqual({ ok: false, error: '文件不是 JSON 对象' })
    const r = themeFromMtb({ schemes: { light: { primary: 'red' }, dark: {} } })
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.error).toContain('schemes.light')
  })

})
