// Registry guarantees (design §9): unknown keys return nothing, figures stay
// quiet (intensity ≤ 0.3, play off, theme auto), sizes follow §9.3, ASCII
// stays within eight lines, figures load lazily, pages never import a figure
// directly, and at most one figure stands on a screen.
import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join, relative, sep } from 'node:path'
import { ART_INTENSITY, artFigureOptions, useArt, type FigurePlacement } from './registry'

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
  return ['signin', 'not-found', 'offline'].map((key) => {
    const entry = useArt(key)
    if (entry?.kind !== 'figure') throw new Error(`placement ${key} is not a figure`)
    return [key, entry]
  })
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
    expect(useArt('signin-mark')).toMatchObject({ kind: 'ascii' })
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
  })

  it('keeps ASCII blocks within eight lines of plain ASCII', () => {
    const mark = useArt('signin-mark')
    if (mark?.kind !== 'ascii') throw new Error('signin-mark is not ASCII')
    const lines = mark.block.split('\n')
    expect(lines.length).toBeLessThanOrEqual(8)
    for (const line of lines) {
      expect(line, 'plain ASCII').toMatch(/^[\x20-\x7e]*$/)
      expect(line, 'no trailing space').not.toMatch(/ $/)
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

  it('rests every figure under reduced motion', () => {
    const css = readFileSync(join(artDir, 'art.css'), 'utf8')
    expect(css).toMatch(/@media\s*\(\s*prefers-reduced-motion\s*:\s*reduce\s*\)\s*\{[^}]*\.art-figure\s*\{[^}]*pointer-events\s*:\s*none/s)
  })

  it('paints lines from --app-text-subtle on --background', () => {
    const css = readFileSync(join(artDir, 'art.css'), 'utf8')
    expect(css).toMatch(/--hairline-(edge|mid)\s*:\s*var\(--app-text-subtle\)/)
    expect(css).toMatch(/--hairline-plate\s*:\s*var\(--background\)/)
  })
})
