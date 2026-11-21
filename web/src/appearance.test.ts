// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { applyAppearance, customStyle, hexToOklch, loadCachedAppearance, oklchToHex, previewCustom, saveCachedAppearance } from './appearance'

afterEach(() => {
  document.documentElement.removeAttribute('data-accent')
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
    expect(css).toContain('--chart-1: var(--primary)')
    expect(css).not.toContain('--chart-2')
    const charts = customStyle('oklch(0.5 0.1 250)', 'oklch(0.7 0.1 250)', ['oklch(0.4 0.1 20)', 'oklch(0.6 0.1 140)', '', '', ''])
    expect(charts).toContain('--chart-1: oklch(0.4 0.1 20)')
    expect(charts).toContain('--chart-2: oklch(0.6 0.1 140)')
    expect(charts).not.toContain('--chart-3')
  })

  it('applies presets as data-accent and custom as a style block', () => {
    applyAppearance({ preset: 'red', accent: '', chartColors: [] }, { light: '', dark: '' })
    expect(document.documentElement.dataset.accent).toBe('red')
    expect(document.getElementById('athena-appearance')).toBeNull()
    applyAppearance({ preset: 'custom', accent: 'oklch(0.5 0.1 250)', chartColors: [] }, { light: 'oklch(0.5 0.1 250)', dark: 'oklch(0.7 0.1 250)' })
    expect(document.documentElement.dataset.accent).toBeUndefined()
    expect(document.getElementById('athena-appearance')?.textContent).toContain('--primary:oklch(0.5 0.1 250)')
    applyAppearance({ preset: 'ink', accent: '', chartColors: [] }, { light: '', dark: '' })
    expect(document.documentElement.dataset.accent).toBe('ink')
    expect(document.getElementById('athena-appearance')).toBeNull()
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

  it('round-trips the per-workspace cache', () => {
    expect(loadCachedAppearance()).toBeNull()
    saveCachedAppearance(
      'acme',
      { preset: 'custom', accent: 'oklch(0.5 0.1 250)', chartColors: [] } as never,
      { light: 'oklch(0.5 0.1 250)', dark: 'oklch(0.7 0.1 250)' },
    )
    expect(loadCachedAppearance()).toEqual({
      slug: 'acme',
      preset: 'custom',
      accent: 'oklch(0.5 0.1 250)',
      chartColors: [],
      light: 'oklch(0.5 0.1 250)',
      dark: 'oklch(0.7 0.1 250)',
    })
  })
})
