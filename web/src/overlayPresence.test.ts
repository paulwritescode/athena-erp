import { describe, expect, it } from 'vitest'
import { claimOverlayTier, registerDrawer, registerSheet, type OverlayDocument } from './components/overlayPresence'

function fakeDoc(): { doc: OverlayDocument; classes: Set<string> } {
  const classes = new Set<string>()
  return {
    classes,
    doc: {
      body: {
        classList: {
          add: (...tokens: string[]) => {
            for (const token of tokens) classes.add(token)
          },
          remove: (...tokens: string[]) => {
            for (const token of tokens) classes.delete(token)
          },
        },
      },
    },
  }
}

describe('overlay presence', () => {
  it('raises the matching body class while open', () => {
    const normal = fakeDoc()
    const release = registerSheet(false, normal.doc)
    expect(normal.classes.has('has-sheet')).toBe(true)
    release()
    expect(normal.classes.has('has-sheet')).toBe(false)

    const wide = fakeDoc()
    const releaseWide = registerSheet(true, wide.doc)
    expect(wide.classes.has('has-sheet-wide')).toBe(true)
    releaseWide()
    expect(wide.classes.has('has-sheet-wide')).toBe(false)
  })

  it('keeps nesting correct and releases once', () => {
    const { doc, classes } = fakeDoc()
    const first = registerSheet(false, doc)
    const second = registerSheet(false, doc)
    first()
    expect(classes.has('has-sheet')).toBe(true)
    first()
    expect(classes.has('has-sheet')).toBe(true)
    second()
    expect(classes.has('has-sheet')).toBe(false)
  })

  it('tracks drawers separately', () => {
    const { doc, classes } = fakeDoc()
    const release = registerDrawer(doc)
    expect(classes.has('has-drawer')).toBe(true)
    release()
    expect(classes.has('has-drawer')).toBe(false)
  })

  it('does nothing without a document', () => {
    expect(() => registerSheet(false)).not.toThrow()
  })
})

describe('overlay tier', () => {
  it('marks the first overlay parent and later ones children', () => {
    const first = claimOverlayTier()
    expect(first.child).toBe(false)
    const second = claimOverlayTier()
    expect(second.child).toBe(true)
    second.release()
    first.release()
  })

  it('hands parent back out after release', () => {
    const first = claimOverlayTier()
    first.release()
    const next = claimOverlayTier()
    expect(next.child).toBe(false)
    next.release()
  })

  it('releases once and never drops below zero', () => {
    const first = claimOverlayTier()
    first.release()
    first.release()
    const next = claimOverlayTier()
    expect(next.child).toBe(false)
    next.release()
  })
})
