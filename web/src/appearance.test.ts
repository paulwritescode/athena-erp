// @vitest-environment jsdom
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, it } from 'vitest'
import {
  ACCENT_PRESETS,
  CHART_PALETTES,
  MATCH_RAMP_VARS,
  MATCH_RAMP_VARS_DARK,
  applyAppearance,
  chartsAreCustom,
  contrastOf,
  customStyle,
  hexToOklch,
  loadCachedAppearance,
  oklchToHex,
  previewCustom,
  PRESET_ACCENTS,
  rampFallbackHex,
  saveCachedAppearance,
} from './appearance'

afterEach(() => {
  document.documentElement.removeAttribute('data-accent')
  document.documentElement.removeAttribute('data-charts')
  document.getElementById('athena-appearance')?.remove()
  localStorage.clear()
})

function parseOklch(s: string): [number, number, number] {
  const m = /^oklch\(([0-9.]+) ([0-9.]+) ([0-9.]+)\)$/.exec(s)
  if (!m) throw new Error(`not oklch: ${s}`)
  return [parseFloat(m[1]), parseFloat(m[2]), parseFloat(m[3])]
}

describe('appearance colors', () => {
  it('converts white and black exactly', () => {
    expect(hexToOklch('#ffffff')).toBe('oklch(1 0 0)')
    expect(hexToOklch('#000000')).toBe('oklch(0 0 0)')
    expect(oklchToHex('oklch(1 0 0)')).toBe('#ffffff')
    expect(oklchToHex('oklch(0 0 0)')).toBe('#000000')
  })

  it('matches the OKLCH reference for pure red', () => {
    // Björn Ottosson's reference: #ff0000 is oklch(0.62796 0.25768 29.234).
    const [l, c, h] = parseOklch(hexToOklch('#ff0000'))
    expect(Math.abs(l - 0.62796)).toBeLessThan(0.001)
    expect(Math.abs(c - 0.25768)).toBeLessThan(0.001)
    expect(Math.abs(h - 29.234)).toBeLessThan(0.01)
  })

  it('round-trips in-gamut colors within a step', () => {
    for (const hex of ['#ff0000', '#7f0019', '#16a34a', '#3b82f6', '#f5f5f4']) {
      const back = oklchToHex(hexToOklch(hex))
      for (let i = 1; i < 7; i += 2) {
        const want = parseInt(hex.slice(i, i + 2), 16)
        const got = parseInt(back.slice(i, i + 2), 16)
        expect(Math.abs(want - got)).toBeLessThanOrEqual(2)
      }
    }
  })

  it('rejects bad input plainly', () => {
    expect(() => hexToOklch('red')).toThrow('hex color')
    expect(() => oklchToHex('red')).toThrow('oklch(')
  })

  it('builds the custom style for both themes with chart fallthrough', () => {
    const css = customStyle('oklch(0.5 0.1 250)', 'oklch(0.7 0.1 250)', ['', '', '', '', ''])
    expect(css).toContain(':root{--primary:oklch(0.5 0.1 250)')
    expect(css).toContain('.dark{--primary:oklch(0.7 0.1 250)')
    // Empty slots fall through to the accent ramp in theme.css.
    expect(css).not.toContain('--chart-1')
    const charts = customStyle('oklch(0.5 0.1 250)', 'oklch(0.7 0.1 250)', ['oklch(0.4 0.1 20)', 'oklch(0.6 0.1 140)', '', '', ''])
    expect(charts).toContain('--chart-1: oklch(0.4 0.1 20)')
    expect(charts).toContain('--chart-2: oklch(0.6 0.1 140)')
    expect(charts).not.toContain('--chart-3')
  })

  it('applies presets as data-accent and custom as a style block', () => {
    applyAppearance({ preset: 'red', accent: '', chartPalette: 'match', chartColors: [] }, { light: '', dark: '' })
    expect(document.documentElement.dataset.accent).toBe('red')
    expect(document.getElementById('athena-appearance')).toBeNull()
    applyAppearance(
      { preset: 'custom', accent: 'oklch(0.5 0.1 250)', chartPalette: 'match', chartColors: [] },
      { light: 'oklch(0.5 0.1 250)', dark: 'oklch(0.7 0.1 250)' },
    )
    expect(document.documentElement.dataset.accent).toBeUndefined()
    expect(document.getElementById('athena-appearance')?.textContent).toContain('--primary:oklch(0.5 0.1 250)')
    applyAppearance({ preset: 'ink', accent: '', chartPalette: 'match', chartColors: [] }, { light: '', dark: '' })
    expect(document.documentElement.dataset.accent).toBe('ink')
    expect(document.getElementById('athena-appearance')).toBeNull()
  })

  it('applies named chart palettes as data-charts, match and custom as none', () => {
    applyAppearance({ preset: 'blue', accent: '', chartPalette: 'ocean', chartColors: [] }, { light: '', dark: '' })
    expect(document.documentElement.dataset.charts).toBe('ocean')
    applyAppearance({ preset: 'blue', accent: '', chartPalette: 'match', chartColors: [] }, { light: '', dark: '' })
    expect(document.documentElement.dataset.charts).toBeUndefined()
    applyAppearance(
      { preset: 'custom', accent: 'oklch(0.5 0.1 250)', chartPalette: 'custom', chartColors: ['oklch(0.4 0.1 20)', '', '', '', ''] },
      { light: 'oklch(0.5 0.1 250)', dark: 'oklch(0.7 0.1 250)' },
    )
    expect(document.documentElement.dataset.charts).toBeUndefined()
    expect(document.getElementById('athena-appearance')?.textContent).toContain('--chart-1: oklch(0.4 0.1 20)')
    // A stale palette never lingers behind a custom style block.
    applyAppearance({ preset: 'ink', accent: '', chartPalette: 'match', chartColors: [] }, { light: '', dark: '' })
    expect(document.documentElement.dataset.charts).toBeUndefined()
  })

  it('previews a passing accent with a lighter dark twin', () => {
    const red = previewCustom('oklch(0.42 0.13 22)')
    expect(red).not.toBeNull()
    expect(red!.passes).toBe(true)
    expect(red!.light).toBe('oklch(0.42 0.13 22)')
    // The dark twin keeps chroma and hue with raised lightness.
    const [l, c, h] = parseOklch(red!.dark)
    expect(c).toBeCloseTo(0.13, 3)
    expect(h).toBeCloseTo(22, 2)
    expect(l).toBeGreaterThan(0.42)
    expect(l).toBeLessThan(1)
  })

  it('flags an accent white text cannot read', () => {
    const pale = previewCustom('oklch(0.9 0.02 250)')
    expect(pale).not.toBeNull()
    expect(pale!.passes).toBe(false)
  })

  it('rejects a broken accent for preview', () => {
    expect(previewCustom('nope')).toBeNull()
  })

  it('derives each accent’s picker ramp from the accent itself', () => {
    // The ink ramp keeps the exact mixes the old hardcoded fallback showed.
    expect([0, 1, 2, 3, 4].map((i) => rampFallbackHex(PRESET_ACCENTS.ink, i))).toEqual(['#18181b', '#3f3f42', '#616164', '#818183', '#9e9ea0'])
    for (const { value } of ACCENT_PRESETS) {
      if (value === 'custom') continue
      const ramp = [0, 1, 2, 3, 4].map((i) => rampFallbackHex(PRESET_ACCENTS[value], i))
      for (const hex of ramp) expect(hex).toMatch(/^#[0-9a-f]{6}$/)
      // Slot 0 is the accent; lightness rises toward white down the ramp.
      expect(ramp[0]).toBe(oklchToHex(PRESET_ACCENTS[value]))
      const lightness = ramp.map((hex) => parseInt(hex.slice(1, 3), 16) + parseInt(hex.slice(3, 5), 16) + parseInt(hex.slice(5, 7), 16))
      for (let i = 1; i < lightness.length; i++) expect(lightness[i]).toBeGreaterThan(lightness[i - 1])
    }
    // Unparseable accents read as ink rather than black.
    expect(rampFallbackHex('nope', 0)).toBe('#18181b')
  })

  it('round-trips the per-workspace cache', () => {
    expect(loadCachedAppearance()).toBeNull()
    saveCachedAppearance(
      'acme',
      { preset: 'custom', accent: 'oklch(0.5 0.1 250)', chartPalette: 'ocean', chartColors: [] } as never,
      { light: 'oklch(0.5 0.1 250)', dark: 'oklch(0.7 0.1 250)' },
    )
    expect(loadCachedAppearance()).toEqual({
      slug: 'acme',
      preset: 'custom',
      accent: 'oklch(0.5 0.1 250)',
      chartPalette: 'ocean',
      chartColors: [],
      light: 'oklch(0.5 0.1 250)',
      dark: 'oklch(0.7 0.1 250)',
    })
    // Caches from before palettes existed read back without one.
    localStorage.setItem('athena-appearance', JSON.stringify({ slug: 'acme', preset: 'ink' }))
    expect(loadCachedAppearance()?.chartPalette).toBe('')
  })
})

describe('preset tokens in theme.css', () => {
  const css = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'theme.css'), 'utf8')

  function block(selector: string): string {
    const m = new RegExp(`^${selector} \\{([^}]*)\\}`, 'm').exec(css)
    if (!m) throw new Error(`missing ${selector}`)
    return m[1]
  }

  function prop(blockText: string, name: string): string {
    const m = new RegExp(`${name}:\\s*([^;]+);`).exec(blockText)
    if (!m) throw new Error(`missing ${name}`)
    return m[1].trim()
  }

  it('lists the nine presets and six chart choices both sides share', () => {
    expect(ACCENT_PRESETS.map((p) => p.value)).toEqual(['ink', 'slate', 'blue', 'sky', 'green', 'amber', 'orange', 'brown', 'red', 'custom'])
    expect(CHART_PALETTES.map((p) => p.value)).toEqual(['match', 'neutral', 'earth', 'ocean', 'forest', 'custom'])
  })

  it('mirrors the light preset primaries for picker fallbacks', () => {
    for (const { value } of ACCENT_PRESETS) {
      if (value === 'custom') continue
      expect(PRESET_ACCENTS[value], value).toBe(prop(block(`\\[data-accent="${value}"\\]`), '--primary'))
    }
  })

  it('passes text contrast on every preset in both themes', () => {
    for (const { value } of ACCENT_PRESETS) {
      if (value === 'custom') continue
      const light = block(`\\[data-accent="${value}"\\]`)
      const dark = block(`\\.dark\\[data-accent="${value}"\\]`)
      const lightRatio = contrastOf(prop(light, '--primary'), prop(light, '--primary-foreground'))
      const darkRatio = contrastOf(prop(dark, '--primary'), prop(dark, '--primary-foreground'))
      expect(lightRatio, `${value} light`).not.toBeNull()
      expect(darkRatio, `${value} dark`).not.toBeNull()
      expect(lightRatio!).toBeGreaterThanOrEqual(4.5)
      expect(darkRatio!).toBeGreaterThanOrEqual(4.5)
    }
  })

  it('declares the match ramp on the scoped accent before the palettes', () => {
    // Custom properties substitute var() where they are declared: without
    // this rule a preview pane scoping data-accent would inherit the saved
    // accent's ramp from :root instead of the draft's.
    const scoped = block('\\[data-accent\\]')
    for (let i = 1; i <= 5; i++) {
      expect(prop(scoped, `--chart-${i}`), `chart-${i}`).toBe(MATCH_RAMP_VARS[`--chart-${i}`])
    }
    expect(css.indexOf('[data-accent] {')).toBeLessThan(css.indexOf('[data-charts='))
    // The dark twin mixes toward black (dark primaries sit near white) and
    // sits before the .dark palettes so they still win ties by order.
    const scopedDark = block('\\.dark\\[data-accent\\]')
    for (let i = 1; i <= 5; i++) {
      expect(prop(scopedDark, `--chart-${i}`), `dark chart-${i}`).toBe(MATCH_RAMP_VARS_DARK[`--chart-${i}`])
    }
    expect(css.indexOf('.dark[data-accent] {')).toBeLessThan(css.indexOf('[data-charts='))
    expect(prop(block('\\.dark'), '--chart-2')).toContain('black')
  })

  it('applies pre-B4b custom charts recorded without a palette', () => {
    expect(chartsAreCustom('', ['oklch(0.4 0.1 20)'])).toBe(true)
    expect(chartsAreCustom('', [])).toBe(false)
    expect(chartsAreCustom('match', [])).toBe(false)
    applyAppearance(
      { preset: 'custom', accent: 'oklch(0.5 0.1 250)', chartPalette: '', chartColors: ['oklch(0.4 0.1 20)', 'oklch(0.6 0.1 140)', '', '', ''] },
      { light: 'oklch(0.5 0.1 250)', dark: 'oklch(0.7 0.1 250)' },
    )
    const cssText = document.getElementById('athena-appearance')?.textContent ?? ''
    expect(cssText).toContain('--chart-1: oklch(0.4 0.1 20)')
    expect(cssText).toContain('--chart-2: oklch(0.6 0.1 140)')
    // A named preset with legacy colors keeps its tokens and gains charts.
    applyAppearance({ preset: 'blue', accent: '', chartPalette: '', chartColors: ['oklch(0.4 0.1 20)', '', '', '', ''] }, { light: '', dark: '' })
    expect(document.documentElement.dataset.accent).toBe('blue')
    const chartsOnly = document.getElementById('athena-appearance')?.textContent ?? ''
    expect(chartsOnly).toContain('--chart-1: oklch(0.4 0.1 20)')
    expect(chartsOnly).not.toContain('--primary')
  })

  it('defines the four named chart palettes in both themes', () => {
    for (const name of ['neutral', 'earth', 'ocean', 'forest']) {
      for (const selector of [`\\[data-charts="${name}"\\]`, `\\.dark\\[data-charts="${name}"\\]`]) {
        const text = block(selector)
        for (let i = 1; i <= 5; i++) {
          expect(prop(text, `--chart-${i}`), `${selector} chart-${i}`).toMatch(/^oklch\(/)
        }
      }
    }
  })

  it('keeps the forced-light preview scope in step with the light blocks', () => {
    for (const { value } of ACCENT_PRESETS) {
      if (value === 'custom') continue
      const light = block(`\\[data-accent="${value}"\\]`)
      const forced = block(`\\.preview-light\\[data-accent="${value}"\\]`)
      for (const name of ['--primary', '--primary-foreground', '--sidebar-primary']) {
        expect(prop(forced, name), `${value} ${name}`).toBe(prop(light, name))
      }
    }
    for (const name of ['neutral', 'earth', 'ocean', 'forest']) {
      const light = block(`\\[data-charts="${name}"\\]`)
      const forced = block(`\\.preview-light\\[data-charts="${name}"\\]`)
      for (let i = 1; i <= 5; i++) {
        expect(prop(forced, `--chart-${i}`), `${name} chart-${i}`).toBe(prop(light, `--chart-${i}`))
      }
    }
    const root = block(':root')
    const forced = block('\\.preview-light')
    for (const name of ['--background', '--foreground', '--card', '--secondary', '--muted-foreground', '--border', '--ring', '--sidebar-accent']) {
      expect(prop(forced, name), name).toBe(prop(root, name))
    }
  })
})
