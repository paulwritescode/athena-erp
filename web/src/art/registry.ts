// The one place for every hairline figure and ASCII block in the desk
// (design §9.7). Pages ask for a placement key through useArt and render it
// with ArtFigure or AsciiMark; they never import a figure themselves.
// Unknown keys return undefined, so a page cannot add art on its own.
// Figures load lazily through dynamic import, so the desk bundle carries no
// figure until a placement mounts one. Every figure plays the same quiet
// part: theme auto, intensity 0.3 or less, play off (§9.4).
import type { Figure as HairlineFigure, HairlineOptions } from '@lucasmarkes/hairline'
import { ATHENA_MARK } from './ascii'

export type FigureName =
  | 'basket'
  | 'drawer'
  | 'format'
  | 'hub'
  | 'lockers'
  | 'padlock'
  | 'plot'
  | 'plug'
  | 'query'
  | 'relay'
  | 'router'
  | 'sieve'
  | 'stack'
  | 'terminal'
  | 'vault'
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
  // Empty states (§9.3, §9.5): every figure matches its page and stays at
  // most 240 wide (120 on phones). At most one art-carrying empty shows per
  // screen (§9.2): the primary list on each page or tab carries the figure
  // and secondary tables keep the icon empty.
  'empty-sieve': { kind: 'figure', figure: 'sieve', load: loader('sieve'), width: 200, mobileWidth: 120 },
  'empty-record-types': { kind: 'figure', figure: 'format', load: loader('format'), width: 200, mobileWidth: 120 },
  'empty-packs': { kind: 'figure', figure: 'basket', load: loader('basket'), width: 200, mobileWidth: 120 },
  'empty-rules': { kind: 'figure', figure: 'terminal', load: loader('terminal'), width: 200, mobileWidth: 120 },
  'empty-runs': { kind: 'figure', figure: 'relay', load: loader('relay'), width: 200, mobileWidth: 120 },
  'empty-files': { kind: 'figure', figure: 'drawer', load: loader('drawer'), width: 200, mobileWidth: 120 },
  'empty-mapping': { kind: 'figure', figure: 'hub', load: loader('hub'), width: 200, mobileWidth: 120 },
  'empty-users': { kind: 'figure', figure: 'lockers', load: loader('lockers'), width: 200, mobileWidth: 120 },
  'empty-roles': { kind: 'figure', figure: 'vault', load: loader('vault'), width: 200, mobileWidth: 120 },
  'empty-sessions': { kind: 'figure', figure: 'router', load: loader('router'), width: 200, mobileWidth: 120 },
  'empty-ledger': { kind: 'figure', figure: 'stack', load: loader('stack'), width: 200, mobileWidth: 120 },
  'empty-reports': { kind: 'figure', figure: 'plot', load: loader('plot'), width: 200, mobileWidth: 120 },
}

export const placementKeys: readonly string[] = Object.keys(placements)

export function useArt(key: string): ArtPlacement | undefined {
  return placements[key]
}

export const ART_INTENSITY = 0.3

export function artFigureOptions(entry: FigurePlacement): HairlineOptions {
  return { intensity: ART_INTENSITY, theme: 'auto', play: false, label: entry.label }
}
