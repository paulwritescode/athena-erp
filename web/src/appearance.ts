// Workspace appearance (B4): presets as data-accent on <html>, custom colors
// as an injected <style> block, cached per Workspace for the first paint.
// The server owns validation and contrast; the desk converts the color
// picker's hex to the oklch() strings the server parses, and applies the
// resolved palette it returns.
import type { Appearance } from './gen/athena/v1/workspace_pb'

export type AppearanceChoice = { preset: string; accent: string; chartPalette: string; chartColors: string[] }
export type ResolvedPalette = { light: string; dark: string }

// The one shared accent list (internal/appearance mirrors it): the nine
// named presets plus custom. They live here (not in the .vue) because the
// token guard reads 'red' or 'green' as a color; as identifiers they select
// token sets, exactly like data-accent="red" which the guard exempts.
export const ACCENT_PRESETS = [
  { value: 'ink', label: 'Ink' },
  { value: 'slate', label: 'Slate' },
  { value: 'blue', label: 'Blue' },
  { value: 'sky', label: 'Sky' },
  { value: 'green', label: 'Green' },
  { value: 'amber', label: 'Amber' },
  { value: 'orange', label: 'Orange' },
  { value: 'brown', label: 'Brown' },
  { value: 'red', label: 'Red' },
  { value: 'custom', label: 'Custom' },
]

// What an empty custom chart slot shows in the picker: the ink accent ramp
// in hex (the exact oklab mixes toward white), so black never poses as a
// choice. It lives here because the token guard forbids hex in components.
export const RAMP_FALLBACK_HEX = ['#18181b', '#3f3f42', '#616164', '#818183', '#9e9ea0']

// The match-accent ramp for custom drafts, which scope no data-accent: the
// same declarations theme.css puts on [data-accent], applied inline so the
// ramp resolves against the pane-local --primary. appearance.test.ts keeps
// the two in step.
export const MATCH_RAMP_VARS: Record<string, string> = {
  '--chart-1': 'var(--primary)',
  '--chart-2': 'color-mix(in oklab, var(--primary) 80%, white)',
  '--chart-3': 'color-mix(in oklab, var(--primary) 64%, white)',
  '--chart-4': 'color-mix(in oklab, var(--primary) 50%, white)',
  '--chart-5': 'color-mix(in oklab, var(--primary) 38%, white)',
}

// Whether stored chart colors apply: explicit custom, or the pre-B4b shape
// (no palette recorded). Mirrors the server's read rule in Resolve.
export function chartsAreCustom(chartPalette: string, chartColors: string[]): boolean {
  return chartPalette === 'custom' || (chartPalette === '' && chartColors.length > 0)
}

// The one shared chart list: a tonal ramp of the accent, four named
// palettes, or custom colors.
export const CHART_PALETTES = [
  { value: 'match', label: 'Match accent', blurb: 'A tonal ramp of the accent.' },
  { value: 'neutral', label: 'Neutral', blurb: 'Quiet grays for formal reports.' },
  { value: 'earth', label: 'Earth', blurb: 'Warm browns and ochres.' },
  { value: 'ocean', label: 'Ocean', blurb: 'Blues and teals.' },
  { value: 'forest', label: 'Forest', blurb: 'Leafy greens.' },
  { value: 'custom', label: 'Custom', blurb: 'Pick each chart color.' },
]

const LIGHT_TEXT = 'oklch(0.985 0 0)'
const DARK_TEXT = 'oklch(0.21 0.006 285.885)'
const STYLE_ID = 'athena-appearance'
const CACHE_KEY = 'athena-appearance'

type Cached = { slug: string; preset: string; accent: string; chartPalette: string; chartColors: string[]; light: string; dark: string }

function trim(n: number): string {
  return String(Math.round(n * 10000) / 10000)
}

// sRGB hex to oklch(), following Björn Ottosson's reference matrices.
export function hexToOklch(hex: string): string {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim())
  if (!m) throw new Error(`Use a hex color like #7f0019, not ${hex}.`)
  const [r8, g8, b8] = [0, 2, 4].map((i) => parseInt(m[1].slice(i, i + 2), 16) / 255)
  const linear = (u: number) => (u <= 0.04045 ? u / 12.92 : Math.pow((u + 0.055) / 1.055, 2.4))
  const r = linear(r8)
  const g = linear(g8)
  const b = linear(b8)
  const l = 0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b
  const mm = 0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b
  const s = 0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b
  const l_ = Math.cbrt(l)
  const m_ = Math.cbrt(mm)
  const s_ = Math.cbrt(s)
  const L = 0.2104542553 * l_ + 0.793617785 * m_ - 0.0040720468 * s_
  const a = 1.9779984951 * l_ - 2.428592205 * m_ + 0.4505937099 * s_
  const bb = 0.0259040371 * l_ + 0.7827717662 * m_ - 0.808675766 * s_
  const C = Math.sqrt(a * a + bb * bb)
  let h = (Math.atan2(bb, a) * 180) / Math.PI
  if (h < 0) h += 360
  return `oklch(${trim(L)} ${trim(C)} ${trim(C < 0.0001 ? 0 : h)})`
}

// oklch() back to sRGB hex, clamped into the gamut, for the color picker.
export function oklchToHex(oklch: string): string {
  const m = /^oklch\(\s*([0-9.]+)(%)?\s+([0-9.]+)\s+(-?[0-9.]+)(deg)?\s*\)$/i.exec(oklch.trim())
  if (!m) throw new Error(`Use oklch(lightness chroma hue), not ${oklch}.`)
  let L = parseFloat(m[1])
  if (m[2]) L /= 100
  const C = parseFloat(m[3])
  const h = (parseFloat(m[4]) * Math.PI) / 180
  const a = C * Math.cos(h)
  const b = C * Math.sin(h)
  const l_ = L + 0.3963377774 * a + 0.2158037573 * b
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b
  const s_ = L - 0.0894841775 * a - 1.291485548 * b
  const l = l_ * l_ * l_
  const mm = m_ * m_ * m_
  const s = s_ * s_ * s_
  const r = 4.0767416621 * l - 3.3077115913 * mm + 0.2309699292 * s
  const g = -1.2684380046 * l + 2.6097574011 * mm - 0.3413193965 * s
  const bb = -0.0041960863 * l - 0.7034186147 * mm + 1.707614701 * s
  const gamma = (u: number) => {
    const v = Math.min(1, Math.max(0, u))
    return Math.round(255 * (v <= 0.0031308 ? 12.92 * v : 1.055 * Math.pow(v, 1 / 2.4) - 0.055))
  }
  const hex = (n: number) => n.toString(16).padStart(2, '0')
  return `#${hex(gamma(r))}${hex(gamma(g))}${hex(gamma(bb))}`
}

// Client mirror of internal/appearance's Resolve, so the preview shows the
// dark accent the server will derive and can warn before saving. The server
// stays authoritative; the browser test checks this against SetAppearance.
type Lab = { l: number; c: number; h: number }

function parseColor(oklch: string): Lab | null {
  const m = /^oklch\(\s*([0-9.]+)(%)?\s+([0-9.]+)\s+(-?[0-9.]+)(deg)?\s*\)$/i.exec(oklch.trim())
  if (!m) return null
  let l = parseFloat(m[1])
  if (m[2]) l /= 100
  return { l, c: parseFloat(m[3]), h: parseFloat(m[4]) }
}

function toRgb(c: Lab): [number, number, number] {
  const h = (c.h * Math.PI) / 180
  const a = c.c * Math.cos(h)
  const b = c.c * Math.sin(h)
  const cube = (x: number) => x * x * x
  const l = cube(c.l + 0.3963377774 * a + 0.2158037573 * b)
  const m = cube(c.l - 0.1055613458 * a - 0.0638541728 * b)
  const s = cube(c.l - 0.0894841775 * a - 1.291485548 * b)
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ]
}

function fitSRGB(c: Lab): Lab {
  const inside = (x: Lab) => {
    const [r, g, b] = toRgb(x)
    return Math.min(r, g, b) >= -1e-10 && Math.max(r, g, b) <= 1 + 1e-10
  }
  if (inside(c)) return { ...c }
  let lo = 0
  let hi = c.c
  for (let i = 0; i < 32; i++) {
    const mid = (lo + hi) / 2
    if (inside({ ...c, c: mid })) lo = mid
    else hi = mid
  }
  return { ...c, c: lo }
}

function luminance(c: Lab): number {
  const clip = (v: number) => Math.max(0, Math.min(1, v))
  const [r, g, b] = toRgb(c).map(clip)
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

const LIGHT_TEXT_LAB: Lab = { l: 0.985, c: 0, h: 0 }
const DARK_TEXT_LAB: Lab = { l: 0.21, c: 0.006, h: 285.885 }

function nearest(c: Lab, dark: boolean): Lab {
  const lightLum = luminance(LIGHT_TEXT_LAB)
  const darkLum = luminance(DARK_TEXT_LAB)
  const pass = (x: Lab) => {
    const lum = luminance(fitSRGB(x))
    return dark ? (lum + 0.05) / (darkLum + 0.05) >= 4.5 : (lightLum + 0.05) / (lum + 0.05) >= 4.5
  }
  if (pass(c)) return { ...c }
  let lo = 0
  let hi = 1000000
  while (lo < hi) {
    const mid = Math.floor((lo + hi) / 2)
    const l = dark ? mid / 1e6 : (1000000 - mid) / 1e6
    if (pass({ ...c, l })) hi = mid
    else lo = mid + 1
  }
  return { ...c, l: (dark ? lo : 1000000 - lo) / 1e6 }
}

function same(a: Lab, b: Lab): boolean {
  return a.l === b.l && a.c === b.c && a.h === b.h
}

export type CustomPreview = { light: string; dark: string; passes: boolean }

export function previewCustom(accent: string): CustomPreview | null {
  const c = parseColor(accent)
  if (!c) return null
  const light = nearest(c, false)
  const passes = same(c, light)
  const dark = fitSRGB(nearest(passes ? c : light, true))
  const show = fitSRGB(c)
  const str = (x: Lab) => `oklch(${trim(x.l)} ${trim(x.c)} ${trim(x.h)})`
  return { light: str(show), dark: str(dark), passes }
}

// WCAG contrast between two oklch() colors, mirroring the server's math
// (fit into sRGB, clipped linear luminance). Null when either side fails
// to parse. The preset test in appearance.test.ts reads theme.css through it.
export function contrastOf(first: string, second: string): number | null {
  const a = parseColor(first)
  const b = parseColor(second)
  if (!a || !b) return null
  const la = luminance(fitSRGB(a))
  const lb = luminance(fitSRGB(b))
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

// The custom <style> block both themes read. The accent carries buttons and
// the selected-nav marker; chart-1 follows it through the accent ramp, and
// empty custom chart slots fall through to that ramp.
export function customStyle(light: string, dark: string, charts: string[]): string {
  const chartRules = charts
    .slice(0, 5)
    .map((value, i) => (value ? ` --chart-${i + 1}: ${value};` : ''))
    .join('')
  // Empty accents (a named preset with legacy custom charts) leave the
  // preset's own tokens alone.
  const lightPrimary = light ? `--primary:${light};--primary-foreground:${LIGHT_TEXT};--sidebar-primary:${light};--sidebar-primary-foreground:${LIGHT_TEXT};` : ''
  const darkPrimary = dark ? `--primary:${dark};--primary-foreground:${DARK_TEXT};--sidebar-primary:${dark};--sidebar-primary-foreground:${DARK_TEXT};` : ''
  return `:root{${lightPrimary}${chartRules}}` + `.dark{${darkPrimary}${chartRules}}`
}

export function applyAppearance(choice: AppearanceChoice, palette: ResolvedPalette): void {
  const root = document.documentElement
  document.getElementById(STYLE_ID)?.remove()
  root.removeAttribute('data-accent')
  root.removeAttribute('data-charts')
  // Inline custom properties from an older application never linger.
  for (const name of ['--primary', '--primary-foreground', '--sidebar-primary', '--sidebar-primary-foreground', '--chart-1', '--chart-2', '--chart-3', '--chart-4', '--chart-5']) {
    root.style.removeProperty(name)
  }
  const customCharts = chartsAreCustom(choice.chartPalette, choice.chartColors)
  if (choice.preset === 'custom') {
    const el = document.createElement('style')
    el.id = STYLE_ID
    el.textContent = customStyle(palette.light || choice.accent, palette.dark || choice.accent, customCharts ? choice.chartColors : [])
    document.head.appendChild(el)
  } else {
    root.dataset.accent = choice.preset || 'ink'
    if (customCharts) {
      const el = document.createElement('style')
      el.id = STYLE_ID
      el.textContent = customStyle('', '', choice.chartColors)
      document.head.appendChild(el)
    }
  }
  if (choice.chartPalette !== 'custom' && choice.chartPalette !== 'match' && choice.chartPalette !== '') {
    root.dataset.charts = choice.chartPalette
  }
}

export function loadCachedAppearance(): Cached | null {
  try {
    const raw = localStorage.getItem(CACHE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<Cached>
    if (typeof parsed.preset !== 'string') return null
    return {
      slug: typeof parsed.slug === 'string' ? parsed.slug : '',
      preset: parsed.preset,
      accent: typeof parsed.accent === 'string' ? parsed.accent : '',
      chartPalette: typeof parsed.chartPalette === 'string' ? parsed.chartPalette : '',
      chartColors: Array.isArray(parsed.chartColors) ? parsed.chartColors.filter((c): c is string => typeof c === 'string') : [],
      light: typeof parsed.light === 'string' ? parsed.light : '',
      dark: typeof parsed.dark === 'string' ? parsed.dark : '',
    }
  } catch {
    return null
  }
}

export function saveCachedAppearance(slug: string, appearance: Appearance, palette: ResolvedPalette): void {
  try {
    localStorage.setItem(
      CACHE_KEY,
      JSON.stringify({ slug, preset: appearance.preset, accent: appearance.accent, chartPalette: appearance.chartPalette, chartColors: appearance.chartColors, light: palette.light, dark: palette.dark } satisfies Cached),
    )
  } catch {
    // A full or blocked store keeps the default look; the next load refetches.
  }
}
