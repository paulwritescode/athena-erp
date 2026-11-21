import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join, relative, sep } from 'node:path'

const root = dirname(fileURLToPath(import.meta.url))

// Full CSS <named-color> list. transparent, currentcolor and inherit are not
// colors and stay allowed everywhere.
const NAMED = new Set(
  'aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue blueviolet brown burlywood cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue darkcyan darkgoldenrod darkgray darkgrey darkgreen darkkhaki darkmagenta darkolivegreen darkorange darkorchid darkred darksalmon darkseagreen darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey dodgerblue firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray grey green greenyellow honeydew hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral lightcyan lightgoldenrodyellow lightgray lightgrey lightgreen lightpink lightsalmon lightseagreen lightskyblue lightslategray lightslategrey lightsteelblue lightyellow lime limegreen linen magenta maroon mediumaquamarine mediumblue mediumorchid mediumpurple mediumseagreen mediumslateblue mediumspringgreen mediumturquoise mediumvioletred midnightblue mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple rebeccapurple red rosybrown royalblue saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue slateblue slategray slategrey snow springgreen steelblue tan teal thistle tomato turquoise violet wheat white whitesmoke yellow yellowgreen'.split(
    ' ',
  ),
)

// Only the palette file itself is exempt, by exact path: a component named
// button-theme.css does not inherit the exemption.
function isExempt(path: string): boolean {
  return relative(root, path).split(sep).join('/') === 'theme.css'
}

function files(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) {
      files(path, out)
    } else if (entry.endsWith('.vue') || entry.endsWith('.css')) {
      out.push(path)
    }
  }
  return out
}

const noComments = (text: string): string => text.replace(/\/\*[\s\S]*?\*\//g, '')

// Attribute values with their binding form. The name must stand alone, so
// data-color never reads as a paint attribute. Spaces around = and unquoted
// values are covered.
function attrValues(body: string, names: string): { bound: boolean; value: string }[] {
  const out: { bound: boolean; value: string }[] = []
  const pattern = new RegExp(`(?<![-\\w])((?:v-bind:|:)?(?:${names}))\\s*=\\s*(?:(["'])([\\s\\S]*?)\\2|([^\\s>]+))`, 'g')
  for (const match of body.matchAll(pattern)) {
    out.push({ bound: match[1].startsWith('v-bind') || match[1].startsWith(':'), value: match[3] ?? match[4] ?? '' })
  }
  return out
}

// String literals inside a binding expression. Unquoted identifiers are
// variables, never colors: :class="red" passes.
function literals(value: string): string[] {
  return [...value.matchAll(/(["'])((?:(?!\1)[\s\S])*)\1/g)].map((match) => match[2])
}

// Only declaration blocks are scanned, so selectors (.red, #abc,
// [data-accent="red"]) never match.
function declarations(css: string): string {
  const blocks = [...css.matchAll(/\{([^{}]*)\}/g)].map((match) => match[1])
  return blocks.length > 0 ? blocks.join('\n') : css
}

// Scannable slices of a .vue file: style blocks, style bindings and plain
// style attributes, class arbitrary values only, paint attributes, and bare
// v-bind objects. Template copy, other attributes and script stay out.
function styleText(path: string, body: string): string[] {
  if (path.endsWith('.css')) return [noComments(body)]
  const blocks: string[] = []
  for (const match of body.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)) blocks.push(match[1])
  for (const attr of attrValues(body, 'style')) {
    if (attr.bound) {
      blocks.push(...literals(attr.value))
    } else {
      blocks.push(attr.value)
    }
  }
  for (const attr of attrValues(body, 'class')) {
    const sources = attr.bound ? literals(attr.value) : [attr.value]
    // Only Tailwind arbitrary values can carry a color; plain class names
    // such as text-red-500 never match.
    for (const source of sources) {
      for (const inner of source.matchAll(/\[([^\]]*)\]/g)) blocks.push(inner[1])
    }
  }
  for (const attr of attrValues(body, 'fill|stroke|color')) {
    if (attr.bound) {
      blocks.push(...literals(attr.value))
    } else {
      blocks.push(attr.value)
    }
  }
  for (const attr of attrValues(body, 'v-bind')) {
    blocks.push(...literals(attr.value))
  }
  return blocks.map(noComments)
}

function violations(path: string, blocks: string[]): string[] {
  const found: string[] = []
  const label = relative(root, path)
  for (const css of blocks) {
    const body = declarations(css)
    for (const match of body.matchAll(/#[0-9a-fA-F]{3,8}\b/g)) found.push(`${label}: hex ${match[0]}`)
    for (const match of body.matchAll(/\brgba?\(/gi)) found.push(`${label}: function ${match[0]}`)
    // Named colors count in value positions: after a colon, comma, paren,
    // quote, equals sign or whitespace. Dots, hashes, hyphens and word
    // characters on either side mean selector, id, property or filename.
    for (const match of body.matchAll(/(?<![-\w.#])([a-zA-Z]+)(?![-\w.])/g)) {
      if (NAMED.has(match[1].toLowerCase())) found.push(`${label}: named color ${match[1]}`)
    }
  }
  return found
}

function checkFile(path: string, body: string): string[] {
  if (isExempt(path)) return []
  return violations(path, styleText(path, body))
}

describe('design tokens', () => {
  it('keeps literal colors out of components (theme.css owns the palette)', () => {
    const bad: string[] = []
    for (const path of files(root)) {
      bad.push(...checkFile(path, readFileSync(path, 'utf8')))
    }
    expect(bad).toEqual([])
  })

  it.each([
    ['hex in css', 'a.css', '.x{color:#abc}', ['hex #abc']],
    ['rgb in css', 'a.css', '.x{color:rgb(1,2,3)}', ['function rgb(']],
    ['RGB uppercase', 'a.css', '.x{color:RGB(1,2,3)}', ['function RGB(']],
    ['named color', 'a.css', '.x{color:red}', ['named color red']],
    ['mixed case Red', 'a.css', '.x{color:Red}', ['named color Red']],
    ['rebeccapurple', 'a.css', '.x{color:rebeccapurple}', ['named color rebeccapurple']],
    ['multi-word value', 'a.css', '.x{border:1px solid blue}', ['named color blue']],
    ['gradient stops', 'a.css', '.x{background:linear-gradient(red,blue)}', ['named color red', 'named color blue']],
    ['button-theme.css is not exempt', 'components/button-theme.css', '.x{color:#abc}', ['hex #abc']],
    ['single-quoted style binding', 'A.vue', `<div :style='{ color: "#abc" }'>`, ['hex #abc']],
    ['double-quoted style binding', 'A.vue', `<div :style="{ color: '#abc' }">`, ['hex #abc']],
    ['plain style attribute', 'A.vue', `<div style="color:#abc">`, ['hex #abc']],
    ['unquoted style value', 'A.vue', `<div style=color:red>`, ['named color red']],
    ['spaces around equals', 'A.vue', `<div :style = "{ color: '#abc' }" class = "bg-[#def]" fill = "#abc">`, ['hex #abc', 'hex #def', 'hex #abc']],
    ['class arbitrary hex', 'A.vue', `<div class="bg-[#abc]">`, ['hex #abc']],
    ['class arbitrary rgb', 'A.vue', `<div class="text-[rgb(1,2,3)]">`, ['function rgb(']],
    ['class arbitrary named', 'A.vue', `<div class="bg-[red]">`, ['named color red']],
    ['svg fill hex', 'A.vue', `<svg><circle fill="#abc"/></svg>`, ['hex #abc']],
    ['svg stroke named', 'A.vue', `<svg><circle stroke="red"/></svg>`, ['named color red']],
    ['svg bound fill', 'A.vue', `<svg><circle :fill="'#abc'"/></svg>`, ['hex #abc']],
    ['bare v-bind object', 'A.vue', `<svg><circle v-bind="{ fill: 'red' }"/></svg>`, ['named color red']],
  ])('fails: %s', (_name, file, body, expected) => {
    const got = checkFile(join(root, file), body).map((v) => v.slice(v.indexOf(': ') + 2))
    expect(got).toEqual(expected)
  })

  it.each([
    ['theme.css is exempt', 'theme.css', '.x{color:#abc}'],
    ['class selector', 'a.css', '.red{color:var(--x)}'],
    ['id selector', 'a.css', '#abc{color:var(--x)}'],
    ['attribute selector', 'a.css', '[data-accent="red"]{color:var(--x)}'],
    ['palette utility class', 'A.vue', `<div class="text-red-500">`],
    ['data attribute', 'A.vue', `<html data-accent="red"><div data-color="red">`],
    ['hyphenated property', 'a.css', '.x{white-space:nowrap}'],
    ['custom property name', 'a.css', '.x{--my-white:var(--x)}'],
    ['comment', 'a.css', '/* #abc red rgb(1,2,3) */.x{}'],
    ['transparent fill', 'A.vue', `<svg><circle fill="none" stroke="currentColor"/></svg>`],
    ['transparent value', 'a.css', '.x{color:transparent}'],
    ['url filename', 'a.css', '.x{background:url(red.png)}'],
    ['identifier binding', 'A.vue', `<div :class="red">`],
  ])('passes: %s', (_name, file, body) => {
    expect(checkFile(join(root, file), body)).toEqual([])
  })
})
