// The CLI banner copies the light final frame (docs/design/ascii-logo.md):
// this test fails while the two differ, whichever side changed.
import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import { LOGO_LIGHT_BLOCK } from './logo-frames'

const artDir = dirname(fileURLToPath(import.meta.url))
const rootDir = dirname(dirname(dirname(artDir)))

describe('CLI banner sync', () => {
  it('matches the light final frame', () => {
    const main = readFileSync(join(rootDir, 'cmd', 'athena', 'main.go'), 'utf8')
    const mark = main.match(/const asciiMark = `([\s\S]*?)`/)
    if (!mark) throw new Error('asciiMark const is missing from cmd/athena/main.go')
    expect(mark[1]).toBe(LOGO_LIGHT_BLOCK)
  })
})
