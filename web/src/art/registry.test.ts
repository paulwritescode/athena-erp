// Registry guarantees (design §9): unknown keys return nothing, figures stay
// quiet (intensity ≤ 0.3, play off, theme auto), sizes follow §9.3, the logo
// draws letter-free frames within eight lines, figures load lazily, pages
// never import a figure or the frames directly, and at most one figure and
// one logo stand on a screen.
import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join, relative, sep } from 'node:path'
import { ART_INTENSITY, artFigureOptions, placementKeys, useArt, type FigurePlacement } from './registry'

const artDir = dirname(fileURLToPath(import.meta.url))
const srcDir = dirname(artDir)

function files(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) {
      files(path, out)
    } else if (entry.endsWith('.vue') || entry.endsWith('.ts')) {
      out.push(path)
    }
  }
  return out
}

function rel(path: string): string {
  return relative(srcDir, path).split(sep).join('/')
}

function figureEntries(): [string, FigurePlacement][] {
  const out: [string, FigurePlacement][] = []
  for (const key of placementKeys) {
    const entry = useArt(key)
    if (entry?.kind === 'figure') out.push([key, entry])
  }
  return out
}

describe('art registry', () => {
  it('returns nothing for unlisted keys', () => {
    expect(useArt('no-such-placement')).toBeUndefined()
    expect(useArt('')).toBeUndefined()
  })

  it('lists the slice-1 placements', () => {
    expect(useArt('signin')).toMatchObject({ kind: 'figure', figure: 'padlock' })
    expect(useArt('not-found')).toMatchObject({ kind: 'figure', figure: 'query' })
    expect(useArt('offline')).toMatchObject({ kind: 'figure', figure: 'plug' })
  })

  it('lists the logo placements, one per waiting surface', () => {
    for (const key of ['signin-logo', 'first-run-logo', 'not-found-logo', 'offline-logo']) {
      expect(useArt(key), key).toMatchObject({ kind: 'logo' })
    }
  })

  it('maps empty states per the section 9.5 meaning table', () => {
    expect(useArt('empty-sieve')).toMatchObject({ kind: 'figure', figure: 'sieve' })
    expect(useArt('empty-files')).toMatchObject({ kind: 'figure', figure: 'drawer' })
    expect(useArt('empty-reports')).toMatchObject({ kind: 'figure', figure: 'plot' })
    for (const key of [
      'empty-record-types', 'empty-packs', 'empty-rules', 'empty-runs', 'empty-mapping',
      'empty-users', 'empty-roles', 'empty-sessions', 'empty-ledger',
    ]) {
      expect(useArt(key)?.kind, key).toBe('figure')
    }
  })

  it('keeps every figure quiet: theme auto, intensity 0.3 or less, play off', () => {
    expect(ART_INTENSITY).toBeLessThanOrEqual(0.3)
    for (const [key, entry] of figureEntries()) {
      const options = artFigureOptions(entry)
      expect(options.theme, key).toBe('auto')
      expect(options.intensity, key).toBeLessThanOrEqual(0.3)
      expect(options.play, key).toBe(false)
    }
  })

  it('sizes follow section 9.3', () => {
    const signin = useArt('signin')
    if (signin?.kind !== 'figure') throw new Error('signin is not a figure')
    expect(signin.width).toBeGreaterThanOrEqual(200)
    expect(signin.width).toBeLessThanOrEqual(280)
    for (const [key, entry] of figureEntries()) {
      expect(entry.width, key).toBeLessThanOrEqual(240)
      expect(entry.mobileWidth, `${key} mobile`).toBeLessThanOrEqual(entry.width)
      expect(entry.mobileWidth, `${key} mobile`).toBeGreaterThan(0)
    }
    const empties = figureEntries().filter(([key]) => key.startsWith('empty-'))
    expect(empties.length).toBeGreaterThan(0)
    for (const [key, entry] of empties) {
      expect(entry.width, key).toBeLessThanOrEqual(240)
      expect(entry.mobileWidth, `${key} mobile`).toBe(120)
    }
  })

  it('draws the logo from letter-free frames within eight lines', () => {
    for (const key of ['signin-logo', 'first-run-logo', 'not-found-logo', 'offline-logo']) {
      const entry = useArt(key)
      if (entry?.kind !== 'logo') throw new Error(`${key} is not a logo`)
      expect(entry.stepMs, `${key} step`).toBeGreaterThanOrEqual(120)
      expect(entry.stepMs, `${key} step`).toBeLessThanOrEqual(240)
      expect(entry.dark.length, `${key} dark frames`).toBe(entry.light.length)
      for (const [theme, frames] of [['light', entry.light], ['dark', entry.dark]] as const) {
        expect(frames.length, `${key}.${theme} frame count`).toBeGreaterThan(1)
        const final = frames[frames.length - 1]!
        expect(final.length, `${key}.${theme} rows`).toBeLessThanOrEqual(8)
        for (const frame of frames) {
          expect(frame.length, `${key}.${theme} keeps full height`).toBe(final.length)
        }
        for (let k = 0; k < frames.length; k++) {
          const frame = frames[k]!
          for (let row = 0; row < frame.length; row++) {
            // Frame k reveals the first k rows and leaves the rest blank.
            expect(frame[row], `${key}.${theme} frame ${k} row ${row}`).toBe(row < k ? final[row] : '')
          }
        }
        for (const line of final) {
          expect(line, 'plain ASCII').toMatch(/^[\x20-\x7e]*$/)
          expect(line, 'no trailing space').not.toMatch(/ $/)
          expect(line, 'never letters').not.toMatch(/[A-Za-z]/)
        }
        expect(final.join(''), `${key}.${theme} draws ink`).toMatch(/#/)
      }
    }
  })

  it('lazy-loads every figure from the hairline package', async () => {
    for (const [key, entry] of figureEntries()) {
      await expect(entry.load(), key).resolves.toEqual(expect.any(Function))
    }
  })

  it('loads figures only through dynamic import inside web/src/art', () => {
    let dynamic = 0
    for (const path of files(srcDir)) {
      if (rel(path).endsWith('.test.ts')) continue
      const body = readFileSync(path, 'utf8')
      if (!body.includes('@lucasmarkes/hairline')) continue
      expect(rel(path), 'hairline use stays in web/src/art').toMatch(/^art\//)
      for (const line of body.split('\n')) {
        if (!line.includes('@lucasmarkes/hairline')) continue
        if (line.includes('import(')) {
          dynamic += 1
        } else {
          expect(line, `${rel(path)} takes the package by type or dynamic import only`).toMatch(/import type/)
        }
      }
    }
    expect(dynamic, 'at least one lazy figure loader').toBeGreaterThan(0)
  })

  it('stands at most one figure on a screen', () => {
    for (const path of files(srcDir)) {
      if (!path.endsWith('.vue') || rel(path).startsWith('art/')) continue
      const uses = readFileSync(path, 'utf8').split('<ArtFigure').length - 1
      expect(uses, `${rel(path)} mounts at most one figure`).toBeLessThanOrEqual(1)
    }
  })

  it('draws at most one logo on a screen', () => {
    for (const path of files(srcDir)) {
      if (!path.endsWith('.vue') || rel(path).startsWith('art/')) continue
      const uses = readFileSync(path, 'utf8').split('<AsciiLogo').length - 1
      expect(uses, `${rel(path)} mounts at most one logo`).toBeLessThanOrEqual(1)
    }
  })

  it('resolves every art key a template names to its kind', () => {
    let keys = 0
    const tag = /<(ArtFigure|AsciiLogo|EmptyState|DataTable)\b((?:"[^"]*"|'[^']*'|[^>"'])*)(\/>|>)/g
    for (const path of files(srcDir)) {
      if (!path.endsWith('.vue') || rel(path).endsWith('.test.ts')) continue
      const body = readFileSync(path, 'utf8')
      for (const match of body.matchAll(tag)) {
        const component = match[1]
        // Static keys only; bound keys such as :art="emptyArt" resolve at
        // runtime with an icon fallback.
        for (const key of match[2].matchAll(/(?:^|\s)(?:art|empty-art|filtered-art|art-key)="([^"]+)"/g)) {
          keys += 1
          const want = component === 'AsciiLogo' ? 'logo' : 'figure'
          expect(useArt(key[1])?.kind, `${rel(path)} ${component} names ${key[1]}`).toBe(want)
        }
      }
    }
    expect(keys, 'at least one template names art').toBeGreaterThan(0)
  })

  it('gives every empty state a figure or an icon', () => {
    // No EmptyState open tag in this tree holds an unquoted `>` (no arrow
    // function), so the tag ends at the first `>` outside quotes.
    const tag = /<EmptyState\b((?:"[^"]*"|'[^']*'|[^>"'])*)(\/>|>)/g
    for (const path of files(srcDir)) {
      if (!path.endsWith('.vue') || rel(path).endsWith('.test.ts')) continue
      const body = readFileSync(path, 'utf8')
      for (const match of body.matchAll(tag)) {
        expect(match[1], `${rel(path)} empty states carry icon or art`).toMatch(/(^|\s):?(icon|art)=/)
      }
    }
  })

  it('rests every figure under reduced motion', () => {
    const css = readFileSync(join(artDir, 'art.css'), 'utf8')
    expect(css).toMatch(/@media\s*\(\s*prefers-reduced-motion\s*:\s*reduce\s*\)\s*\{[^}]*\.art-figure\s*\{[^}]*pointer-events\s*:\s*none/s)
  })

  it('rests the logo draw under reduced motion', () => {
    const css = readFileSync(join(artDir, 'art.css'), 'utf8')
    expect(css).toMatch(/@media\s*\(\s*prefers-reduced-motion\s*:\s*reduce\s*\)\s*\{[^}]*\.ascii-logo-line-new\s*\{[^}]*animation\s*:\s*none/s)
  })

  it('fades new logo rows through opacity only', () => {
    const css = readFileSync(join(artDir, 'art.css'), 'utf8')
    const frames = css.match(/@keyframes ascii-logo-in\s*\{([\s\S]*?)\n\}/)
    if (!frames) throw new Error('ascii-logo-in keyframes are missing')
    expect(frames[1]).toMatch(/opacity/)
    expect(frames[1]).not.toMatch(/transform|color|width|height|margin|padding/)
  })

  it('paints lines from --app-text-subtle on --background', () => {
    const css = readFileSync(join(artDir, 'art.css'), 'utf8')
    expect(css).toMatch(/--hairline-(edge|mid)\s*:\s*var\(--app-text-subtle\)/)
    expect(css).toMatch(/--hairline-plate\s*:\s*var\(--background\)/)
  })
})
