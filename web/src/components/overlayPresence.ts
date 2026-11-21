import { ref } from 'vue'

// Tracks open sheet-class overlays so chrome can react: toasts move beside
// an open right sheet (body classes, style.css) and the phone navigation
// hides under any bottom drawer. DetailSheet raises has-sheet(-wide);
// drawers rendered below 1024 px raise has-drawer. Counters keep nesting
// correct; the document is injectable so the logic is unit-testable.
// sheetDepth mirrors the count reactively for the Toaster position: below
// 1024 px the free lane beside a sheet is too narrow, so toasts go top.
export const sheetDepth = ref(0)
//
// claimOverlayTier hands out the §16 stacking tier: the first overlay sits
// at 50 and anything opened while one is already open is a child at 60, so
// a child always paints above its parent's overlay. The flag is captured
// when claimed and stays put; at most two levels stack, so a boolean is
// enough. Every modal overlay with its own dim layer claims exactly once
// while open: BottomDrawer claims for itself, DetailSheet claims for its
// right-sheet half only (its phone half is a BottomDrawer and claims
// itself), and desktop dialogs sit at 60 unconditionally.
let depth = 0

export function claimOverlayTier(): { child: boolean; release: () => void } {
  const child = depth > 0
  depth += 1
  let done = false
  return {
    child,
    release() {
      if (done) return
      done = true
      depth = Math.max(0, depth - 1)
    },
  }
}

const counts = new Map<string, number>()

export type OverlayDocument = { body: { classList: { add: (...tokens: string[]) => void; remove: (...tokens: string[]) => void } } }

function bump(cls: string, delta: number, doc: OverlayDocument): void {
  const next = (counts.get(cls) ?? 0) + delta
  if (next <= 0) {
    counts.delete(cls)
    doc.body.classList.remove(cls)
  } else {
    counts.set(cls, next)
    doc.body.classList.add(cls)
  }
}

function track(cls: string, doc?: OverlayDocument | undefined): () => void {
  const root = doc ?? (typeof document === 'undefined' ? undefined : document)
  if (!root) return () => {}
  bump(cls, 1, root)
  let done = false
  return () => {
    if (done) return
    done = true
    bump(cls, -1, root)
  }
}

export function registerSheet(wide: boolean, doc?: OverlayDocument | undefined): () => void {
  sheetDepth.value += 1
  const release = track(wide ? 'has-sheet-wide' : 'has-sheet', doc)
  return () => {
    sheetDepth.value = Math.max(0, sheetDepth.value - 1)
    release()
  }
}

export function registerDrawer(doc?: OverlayDocument | undefined): () => void {
  return track('has-drawer', doc)
}
