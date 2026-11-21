// The one place for every hairline figure and ASCII block in the desk
// (design §9.7). Pages ask for a placement key through useArt and render it
// with ArtFigure or AsciiMark; they never import a figure themselves.
// Unknown keys return undefined, so a page cannot add art on its own.
// Figures load lazily through dynamic import, so the desk bundle carries no
// figure until a placement mounts one. Every figure plays the same quiet
// part: theme auto, intensity 0.3 or less, play off (§9.4).
import type { Figure as HairlineFigure, HairlineOptions } from '@lucasmarkes/hairline'
import { ATHENA_MARK } from './ascii'

export type FigureName = 'padlock' | 'plug' | 'query'
export type FigureFn = (el: HTMLElement, options?: HairlineOptions) => HairlineFigure
export type FigureLoader = () => Promise<FigureFn>

export interface FigurePlacement {
  kind: 'figure'
  figure: FigureName
  load: FigureLoader
  /** Figure width in px on desktop and tablet (§9.3). */
  width: number
  /** Figure width in px on phones (§3.1, below 640 px). */
  mobileWidth: number
  /** Absent for decorative figures, which render aria-hidden (§9.5). */
  label?: string
}

export interface AsciiPlacement {
  kind: 'ascii'
  block: string
}

export type ArtPlacement = FigurePlacement | AsciiPlacement

function loader(name: FigureName): FigureLoader {
  return () => import('@lucasmarkes/hairline').then((figures) => figures[name])
}

const placements: Record<string, ArtPlacement> = {
  // Padlock for sign-in (§9.5), at the sign-in size (§9.3: 200 to 280).
  signin: { kind: 'figure', figure: 'padlock', load: loader('padlock'), width: 240, mobileWidth: 160 },
  'signin-mark': { kind: 'ascii', block: ATHENA_MARK },
  // A question mark for the page that does not exist; a plug that stops
  // short of its socket for the offline page. Both match their page (§9.5)
  // and stay within the empty-state cap (§9.3: at most 240).
  'not-found': { kind: 'figure', figure: 'query', load: loader('query'), width: 200, mobileWidth: 160 },
  offline: { kind: 'figure', figure: 'plug', load: loader('plug'), width: 200, mobileWidth: 160 },
}

export function useArt(key: string): ArtPlacement | undefined {
  return placements[key]
}

export const ART_INTENSITY = 0.3

export function artFigureOptions(entry: FigurePlacement): HairlineOptions {
  return { intensity: ART_INTENSITY, theme: 'auto', play: false, label: entry.label }
}
