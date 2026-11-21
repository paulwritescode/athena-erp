import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join, relative, sep } from 'node:path'
import { parse as parseSFC } from 'vue/compiler-sfc'

const root = dirname(fileURLToPath(import.meta.url))

// Design tokens guard: literal colors belong in theme.css only. The check is
// structural, not regex-on-text. `@vue/compiler-sfc` parses each `.vue` file
// into blocks and the template block into the real template AST, which is
// walked for paint attributes and bindings by exact name (fragments without
// a template block are wrapped so the same walk applies). A brace and
// string-aware CSS scanner reads every declaration value in every rule,
// nested or not; binding expressions are tokenized with nesting, string,
// escape and template-${} awareness. (@vue/compiler-dom and postcss are
// transitive-only while web/package.json sits outside the B2 file scope, so
// expression and CSS scanning are purpose-built here and covered by the
// fixtures below. A regex literal holding braces next to a data-* key in one
// v-bind object could mis-split, a shape that appears nowhere in this tree.)
//
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

// --- CSS scanning: every declaration value in every rule, nested or not.
// Selectors, at-rule preludes, property names and comments never match
// because they never enter a value.

function stripCssComments(css: string): string {
  let out = ''
  let i = 0
  while (i < css.length) {
    const c = css[i]
    if (c === '"' || c === "'") {
      // Strings are copied whole; comment markers inside them are text.
      const end = readQuoted(css, i, c)
      out += css.slice(i, end)
      i = end
    } else if (c === '/' && css[i + 1] === '*') {
      const end = css.indexOf('*/', i + 2)
      i = end === -1 ? css.length : end + 2
    } else {
      out += c
      i += 1
    }
  }
  return out
}

// Index just past the string starting at i (i points at the quote).
function readQuoted(s: string, i: number, quote: string): number {
  let j = i + 1
  while (j < s.length) {
    if (s[j] === '\\') {
      j += 2
    } else if (s[j] === quote) {
      return j + 1
    } else {
      j += 1
    }
  }
  return j
}

// Split text on a separator that sits at nesting depth zero, skipping over
// strings and paren/bracket groups. Used for top-level ; and , splits.
function splitTopLevel(text: string, separator: string): string[] {
  const parts: string[] = []
  let depth = 0
  let start = 0
  let i = 0
  while (i < text.length) {
    const c = text[i]
    if (c === '"' || c === "'") {
      i = readQuoted(text, i, c)
    } else if (c === '`') {
      i = readTemplate(text, i)
    } else if (c === '(' || c === '[' || c === '{') {
      depth += 1
      i += 1
    } else if (c === ')' || c === ']' || c === '}') {
      depth = Math.max(0, depth - 1)
      i += 1
    } else if (c === separator && depth === 0) {
      parts.push(text.slice(start, i))
      start = i + 1
      i += 1
    } else {
      i += 1
    }
  }
  parts.push(text.slice(start))
  return parts
}

// Index just past the template literal starting at i, consuming ${}
// expressions with full nesting so braces inside them never unbalance us.
function readTemplate(s: string, i: number): number {
  let j = i + 1
  while (j < s.length) {
    const c = s[j]
    if (c === '\\') {
      j += 2
    } else if (c === '`') {
      return j + 1
    } else if (c === '$' && s[j + 1] === '{') {
      j = readBalanced(s, j + 2)
    } else {
      j += 1
    }
  }
  return j
}

// Index just past the } balancing the ${ whose body starts at i.
function readBalanced(s: string, i: number): number {
  let depth = 1
  let j = i
  while (j < s.length && depth > 0) {
    const c = s[j]
    if (c === '"' || c === "'") {
      j = readQuoted(s, j, c)
    } else if (c === '`') {
      j = readTemplate(s, j)
    } else if (c === '{') {
      depth += 1
      j += 1
    } else if (c === '}') {
      depth -= 1
      j += 1
    } else {
      j += 1
    }
  }
  return j
}

// Values of the declarations in text, which holds declaration list syntax
// (a rule body with nested rules already expanded, or a style attribute).
function declarationValues(text: string): string[] {
  const values: string[] = []
  for (const chunk of splitTopLevel(text, ';')) {
    const colon = topLevelColon(chunk)
    if (colon !== -1) values.push(chunk.slice(colon + 1))
  }
  return values
}

function topLevelColon(text: string): number {
  let depth = 0
  let i = 0
  while (i < text.length) {
    const c = text[i]
    if (c === '"' || c === "'") {
      i = readQuoted(text, i, c)
    } else if (c === '`') {
      i = readTemplate(text, i)
    } else if (c === '(' || c === '[' || c === '{') {
      depth += 1
      i += 1
    } else if (c === ')' || c === ']' || c === '}') {
      depth = Math.max(0, depth - 1)
      i += 1
    } else if (c === ':' && depth === 0) {
      return i
    } else {
      i += 1
    }
  }
  return -1
}

// Collect declaration values from css, descending into nested rules and
// at-rule blocks. depth counts rule braces; prelude text is skipped.
function ruleValues(css: string): string[] {
  const values: string[] = []
  const text = stripCssComments(css)
  collectBlock(text, 0, values)
  return values
}

function collectBlock(s: string, i: number, values: string[]): number {
  let j = i
  let run = ''
  const flush = () => {
    values.push(...declarationValues(run))
    run = ''
  }
  while (j < s.length) {
    const c = s[j]
    if (c === '"' || c === "'") {
      const end = readQuoted(s, j, c)
      run += s.slice(j, end)
      j = end
    } else if (c === '`') {
      const end = readTemplate(s, j)
      run += s.slice(j, end)
      j = end
    } else if (c === '{') {
      // A nested rule: declarations before the last semicolon still count;
      // the rest of the run is its selector.
      const semi = run.lastIndexOf(';')
      if (semi !== -1) values.push(...declarationValues(run.slice(0, semi + 1)))
      run = ''
      j = collectBlock(s, j + 1, values)
    } else if (c === '}') {
      flush()
      return j + 1
    } else {
      run += c
      j += 1
    }
  }
  flush()
  return j
}

// --- Template scanning: the real template AST from compiler-sfc, walked for
// paint attributes and bindings by exact name, so data-color never reads as
// a paint attribute and comments never match.

type AstProp = { bound: boolean; arg: string; value: string | null }

function astProps(ast: unknown): AstProp[] {
  const out: AstProp[] = []
  const visit = (node: unknown) => {
    if (!node || typeof node !== 'object') return
    const record = node as Record<string, unknown>
    if (Array.isArray(record.props)) {
      const tag = typeof record.tag === 'string' ? record.tag.toLowerCase() : ''
      // Fragments re-parsed inside a wrapper keep their script/style tags as
      // plain elements; their text is code, not attributes.
      if (tag !== 'script' && tag !== 'style') {
        for (const prop of record.props as Record<string, unknown>[]) {
          if (!prop || typeof prop !== 'object' || typeof prop.name !== 'string') continue
          if ('value' in prop) {
            const content = prop.value as { content?: unknown } | undefined
            out.push({ bound: false, arg: prop.name, value: typeof content?.content === 'string' ? content.content : null })
          } else if (prop.name === 'bind') {
            const arg = prop.arg as { content?: unknown; isStatic?: unknown } | null | undefined
            if (arg != null && (arg.isStatic === false || typeof arg.content !== 'string')) continue
            const exp = prop.exp as { content?: unknown } | null | undefined
            out.push({ bound: true, arg: typeof arg?.content === 'string' ? arg.content : '', value: typeof exp?.content === 'string' ? exp.content : null })
          }
        }
      }
    }
    if (Array.isArray(record.children)) {
      for (const child of record.children) visit(child)
    }
  }
  visit(ast)
  return out
}

// Only literals that can reach a paint position count. A ternary condition
// is a test, never the value, so conditions blank out (branches scan).
// && and || operands can all be the value, so they all scan.
type Unit = { text: string; group: string | null }

function splitUnits(code: string): Unit[] {
  const out: Unit[] = []
  let i = 0
  let start = 0
  const flush = (to: number) => {
    if (to > start) out.push({ text: code.slice(start, to), group: null })
  }
  while (i < code.length) {
    const c = code[i]
    if (c === '"' || c === "'") {
      const end = readQuoted(code, i, c)
      flush(i)
      out.push({ text: code.slice(i, end), group: 'str' })
      i = end
      start = i
    } else if (c === '`') {
      const end = readTemplate(code, i)
      flush(i)
      out.push({ text: code.slice(i, end), group: 'str' })
      i = end
      start = i
    } else if (c === '(' || c === '[' || c === '{') {
      const end = groupEnd(code, i)
      flush(i)
      out.push({ text: code.slice(i, end), group: c })
      i = end
      start = i
    } else {
      i += 1
    }
  }
  flush(code.length)
  return out
}

// Index just past the group opening at i, strings and nesting honored.
function groupEnd(code: string, i: number): number {
  let depth = 0
  let j = i
  while (j < code.length) {
    const c = code[j]
    if (c === '"' || c === "'") {
      j = readQuoted(code, j, c)
    } else if (c === '`') {
      j = readTemplate(code, j)
    } else if (c === '(' || c === '[' || c === '{') {
      depth += 1
      j += 1
    } else if (c === ')' || c === ']' || c === '}') {
      depth -= 1
      j += 1
      if (depth === 0) return j
    } else {
      j += 1
    }
  }
  return j
}

function stripConditions(code: string): string {
  let out = ''
  let seg = ''
  let ternDepth = 0
  const flushSeg = (blank: boolean) => {
    out += blank ? ' '.repeat(seg.length) : seg
    seg = ''
  }
  const pushText = (text: string) => {
    let k = 0
    while (k < text.length) {
      const c = text[k]
      if (c === '?' && text[k + 1] !== '.' && text[k + 1] !== '?') {
        flushSeg(true)
        ternDepth += 1
        out += '?'
        k += 1
      } else if (c === ':' && ternDepth > 0) {
        flushSeg(false)
        ternDepth -= 1
        out += ':'
        k += 1
      } else if ((c === ',' || c === ';') && ternDepth === 0) {
        flushSeg(false)
        out += c
        k += 1
      } else {
        seg += c
        k += 1
      }
    }
  }
  for (const unit of splitUnits(code)) {
    if (unit.group === null) {
      pushText(unit.text)
    } else if (unit.group === 'str') {
      seg += unit.text
    } else {
      seg += unit.text[0] + stripConditions(unit.text.slice(1, -1)) + (unit.text.length > 1 ? unit.text[unit.text.length - 1] : '')
    }
  }
  flushSeg(false)
  return out
}

function valueLiterals(rawCode: string): string[] {
  return stringLiterals(stripConditions(cleanJs(rawCode)))
}

// Every string literal in a binding expression, single/double/backtick
// quoted, escapes and template ${} honored. Unquoted identifiers are
// variables, never colors.
function stringLiterals(expr: string): string[] {
  const out: string[] = []
  let i = 0
  while (i < expr.length) {
    const c = expr[i]
    if (c === '"' || c === "'") {
      const end = readQuoted(expr, i, c)
      out.push(expr.slice(i + 1, end - 1))
      i = end
    } else if (c === '`') {
      const end = readTemplate(expr, i)
      out.push(expr.slice(i + 1, end - 1))
      i = end
    } else {
      i += 1
    }
  }
  return out
}

// Values of an object binding's pairs, skipping data-* keys: the key is a
// name, never a color, and data-* values belong to the page, not the paint.
function objectValues(rawExpr: string): string[] {
  const expr = cleanJs(rawExpr)
  const trimmed = expr.trim()
  if (!trimmed.startsWith('{') || !trimmed.endsWith('}')) return []
  const values: string[] = []
  for (const pair of splitTopLevel(trimmed.slice(1, -1), ',')) {
    const colon = topLevelColon(pair)
    if (colon === -1) {
      values.push(...stringLiterals(pair))
      continue
    }
    if (stripKey(pair.slice(0, colon)).toLowerCase().startsWith('data-')) continue
    values.push(...valueLiterals(pair.slice(colon + 1)))
  }
  return values
}

function stripKey(key: string): string {
  return key
    .trim()
    .replace(/^\[|\]$/g, '')
    .trim()
    .replace(/^["']|["']$/g, '')
}

type ObjectSpan = { inner: string; from: number; to: number }

// Top-level brace spans of an expression, for bindings that choose between
// objects (`cond ? { fill: '#abc' } : {}`). Strings and nesting honored.
function objectSpans(exp: string): ObjectSpan[] {
  const spans: ObjectSpan[] = []
  let i = 0
  while (i < exp.length) {
    const c = exp[i]
    if (c === '"' || c === "'") {
      i = readQuoted(exp, i, c)
    } else if (c === '`') {
      i = readTemplate(exp, i)
    } else if (c === '{') {
      const end = readBalanced(exp, i + 1)
      spans.push({ inner: exp.slice(i + 1, end - 1), from: i, to: end })
      i = end
    } else {
      i += 1
    }
  }
  return spans
}

function blankSpans(exp: string, spans: ObjectSpan[]): string {
  let out = ''
  let at = 0
  for (const span of spans) {
    out += exp.slice(at, span.from) + ' '.repeat(span.to - span.from)
    at = span.to
  }
  return out + exp.slice(at)
}

// Attributes whose values are paint, plain or bound.
const PAINT_ATTRS = new Set(['fill', 'stroke', 'color', 'stop-color', 'flood-color', 'lighting-color'])

// Values of a bare v-bind object. Only style, class and paint keys can carry
// a color; every other key (title, data-*, id, ...) is text.
function bindObjectValues(rawExp: string): string[] {
  const exp = cleanJs(rawExp)
  const values: string[] = []
  const spans = objectSpans(exp)
  for (const span of spans) {
    for (const pair of splitTopLevel(span.inner, ',')) {
      const colon = topLevelColon(pair)
      if (colon === -1) {
        values.push(...valueLiterals(pair))
        continue
      }
      const key = stripKey(pair.slice(0, colon)).toLowerCase()
      const val = pair.slice(colon + 1)
      if (key === 'style') {
        values.push(...styleValueLiterals(val))
      } else if (key === 'class') {
        for (const lit of valueLiterals(val)) values.push(...arbitraryValues(lit))
      } else if (PAINT_ATTRS.has(key)) {
        values.push(...valueLiterals(val))
      }
    }
  }
  // Whatever sits outside objects is code, but a stray string still scans.
  values.push(...valueLiterals(blankSpans(exp, spans)))
  return values
}

// Values of a :style binding: objects scan pair by pair, anything else scans
// as literals.
function styleExpValues(rawExp: string): string[] {
  const exp = cleanJs(rawExp)
  const values: string[] = []
  const spans = objectSpans(exp)
  for (const span of spans) values.push(...objectValues(`{${span.inner}}`))
  values.push(...valueLiterals(blankSpans(exp, spans)))
  return values
}

function styleValueLiterals(val: string): string[] {
  const trimmed = val.trim()
  if (trimmed.startsWith('{') && trimmed.endsWith('}')) return objectValues(val)
  const out: string[] = []
  for (const lit of valueLiterals(val)) {
    const decls = declarationValues(lit)
    if (decls.length > 0) out.push(...decls)
    else out.push(lit)
  }
  return out
}

// Tailwind arbitrary values: outermost bracket pairs, with nesting and
// quotes honored so a value containing brackets stays whole.
function arbitraryValues(s: string): string[] {
  const out: string[] = []
  let i = 0
  while (i < s.length) {
    const open = s.indexOf('[', i)
    if (open === -1) break
    let depth = 0
    let j = open
    let closed = -1
    while (j < s.length) {
      const c = s[j]
      if (c === '"' || c === "'") {
        j = readQuoted(s, j, c)
      } else if (c === '[') {
        depth += 1
        j += 1
      } else if (c === ']') {
        depth -= 1
        j += 1
        if (depth === 0) {
          closed = j
          break
        }
      } else {
        j += 1
      }
    }
    if (closed === -1) break
    // Tailwind spells spaces as underscores inside arbitrary values.
    out.push(s.slice(open + 1, closed - 1).replace(/_/g, ' '))
    i = closed
  }
  return out
}

// Scannable value strings of a template AST: style bindings and plain style
// attributes, class arbitrary values only, paint attributes, and bare v-bind
// objects. Template copy, other attributes, data-* and script stay out.
function astValues(ast: unknown): string[] {
  const values: string[] = []
  for (const attr of astProps(ast)) {
    if (attr.value === null) continue
    if (attr.arg === 'style') {
      if (!attr.bound) values.push(...declarationValues(attr.value))
      else values.push(...styleExpValues(attr.value))
    } else if (attr.arg === 'class') {
      const sources = attr.bound ? valueLiterals(attr.value) : [attr.value]
      for (const source of sources) values.push(...arbitraryValues(source))
    } else if (PAINT_ATTRS.has(attr.arg)) {
      if (!attr.bound) values.push(attr.value)
      else values.push(...valueLiterals(attr.value))
    } else if (attr.bound && attr.arg === '') {
      values.push(...bindObjectValues(attr.value))
    }
  }
  return values
}

// A url() holds a resource or a fragment reference, never a color.
function stripUrls(value: string): string {
  let out = ''
  let i = 0
  while (i < value.length) {
    if (value.slice(i, i + 4).toLowerCase() === 'url(') {
      let j = i + 4
      let depth = 1
      while (j < value.length && depth > 0) {
        const c = value[j]
        if (c === '"' || c === "'") {
          j = readQuoted(value, j, c)
        } else if (c === '(') {
          depth += 1
          j += 1
        } else if (c === ')') {
          depth -= 1
          j += 1
        } else {
          j += 1
        }
      }
      out += 'url()'
      i = j
    } else {
      out += value[i]
      i += 1
    }
  }
  return out
}

function violations(label: string, values: string[]): string[] {
  const found: string[] = []
  for (const raw of values) {
    const value = stripUrls(raw)
    for (const match of value.matchAll(/#[0-9a-fA-F]{3,8}\b/g)) found.push(`${label}: hex ${match[0]}`)
    for (const match of value.matchAll(/\brgba?\(/gi)) found.push(`${label}: function ${match[0]}`)
    // Named colors count in value positions: $ introduces a variable, dots,
    // hashes, hyphens and word characters on either side mean property,
    // filename or function name.
    for (const match of value.matchAll(/(?<![-\w.#$])([a-zA-Z]+)(?![-\w.])/g)) {
      if (NAMED.has(match[1].toLowerCase())) found.push(`${label}: named color ${match[1]}`)
    }
  }
  return found
}

// Script strings that hold a color are violations too: a palette object in
// <script setup> reaches paint through :style just like a binding literal.
// Files with a legitimate reason list here; the list is empty today.
const SCRIPT_EXCEPTIONS = new Set<string>([])

// JavaScript only (never CSS): comments blanked, regex literals blanked, so
// neither confuses the scanners. A / starts a regex unless the previous
// significant token is an operand (identifier, number, string, regex, )
// or ]); keywords that take values allow one. Regex contents are patterns,
// never paint.
const REGEX_AFTER_KEYWORDS = new Set([
  'return', 'typeof', 'case', 'in', 'of', 'new', 'do', 'else', 'yield', 'await', 'instanceof', 'throw', 'delete', 'void',
])

function isWordChar(c: string | undefined): boolean {
  return c !== undefined && /[\w$#]/.test(c)
}

// Index just past the regex starting at i, or -1 when it never terminates.
function readRegex(code: string, i: number): number {
  let j = i + 1
  let inClass = false
  while (j < code.length) {
    const c = code[j]
    if (c === '\\') {
      j += 2
    } else if (c === '[') {
      inClass = true
      j += 1
    } else if (c === ']') {
      inClass = false
      j += 1
    } else if (c === '/' && !inClass) {
      j += 1
      while (j < code.length && /[a-z]/.test(code[j])) j += 1
      return j
    } else if (c === '\n') {
      return -1
    } else {
      j += 1
    }
  }
  return -1
}

function cleanJs(code: string): string {
  let out = ''
  let i = 0
  let prevOperand = false
  let prevWord = ''
  const blanks = (from: number, to: number) => {
    out += ' '.repeat(to - from)
  }
  while (i < code.length) {
    const c = code[i]
    if (c === '"' || c === "'") {
      const end = readQuoted(code, i, c)
      out += code.slice(i, end)
      i = end
      prevOperand = true
      prevWord = ''
    } else if (c === '`') {
      const end = readTemplate(code, i)
      out += code.slice(i, end)
      i = end
      prevOperand = true
      prevWord = ''
    } else if (isWordChar(c)) {
      let j = i + 1
      while (j < code.length && isWordChar(code[j])) j += 1
      const word = code.slice(i, j)
      out += word
      i = j
      prevWord = word
      prevOperand = !REGEX_AFTER_KEYWORDS.has(word)
    } else if (c === '/' && code[i + 1] === '/') {
      const end = code.indexOf('\n', i + 2)
      const to = end === -1 ? code.length : end
      blanks(i, to)
      i = to
    } else if (c === '/' && code[i + 1] === '*') {
      const end = code.indexOf('*/', i + 2)
      const to = end === -1 ? code.length : end + 2
      blanks(i, to)
      i = to
    } else if ((c === '+' || c === '-') && code[i + 1] === c) {
      out += c + c
      i += 2
      prevOperand = true
      prevWord = ''
    } else if (c === '/') {
      const end = !prevOperand || REGEX_AFTER_KEYWORDS.has(prevWord) ? readRegex(code, i) : -1
      if (end === -1) {
        out += c
        i += 1
        prevOperand = false
        prevWord = ''
      } else {
        blanks(i, end)
        i = end
        prevOperand = true
        prevWord = ''
      }
    } else {
      out += c
      if (c === ')' || c === ']') {
        prevOperand = true
        prevWord = ''
      } else if (!/\s/.test(c)) {
        prevOperand = false
        prevWord = ''
      }
      i += 1
    }
  }
  return out
}

function scriptValues(content: string): string[] {
  return stringLiterals(cleanJs(content))
}

function checkFile(path: string, body: string): string[] {
  if (isExempt(path)) return []
  const label = relative(root, path)
  if (path.endsWith('.css')) return violations(label, ruleValues(body))
  const { descriptor, errors } = parseSFC(body, { filename: path })
  const hasBlocks = descriptor.template !== null || descriptor.styles.length > 0 || descriptor.script !== null || descriptor.scriptSetup !== null
  if (errors.length > 0 && hasBlocks) return [`${label}: template error ${errors[0].message}`]
  const values: string[] = []
  for (const style of descriptor.styles) {
    if (!style.src) values.push(...ruleValues(style.content))
  }
  if (!SCRIPT_EXCEPTIONS.has(label)) {
    for (const script of [descriptor.script, descriptor.scriptSetup]) {
      if (script && !script.src) values.push(...scriptValues(script.content))
    }
  }
  if (descriptor.template?.ast) {
    values.push(...astValues(descriptor.template.ast))
  } else if (!descriptor.template) {
    // A fragment without a template block (fixtures): wrap it so the same
    // AST walk applies. Script and style tags inside stay unscanned.
    const wrapped = parseSFC(`<template>${body}</template>`, { filename: path })
    if (wrapped.descriptor.template?.ast) values.push(...astValues(wrapped.descriptor.template.ast))
  }
  return violations(label, values)
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
    ['backtick style binding', 'A.vue', '<div :style="{ fill: `#abc` }">', ['hex #abc']],
    ['backtick with expression', 'A.vue', '<div :style="`color:${c ? `#abc` : `#def`}`">', ['hex #abc', 'hex #def']],
    ['nested rule declarations', 'a.css', '.a{color:#abc;&:hover{color:#def}}', ['hex #abc', 'hex #def']],
    ['nested selector skipped, inner caught', 'a.css', '.a{.red{color:#abc}}', ['hex #abc']],
    ['keyframes declarations', 'a.css', '@keyframes x{0%{color:red}}', ['named color red']],
    ['v-bind skips data key, flags fill', 'A.vue', `<svg><circle v-bind="{ 'data-color': 'red', fill: '#abc' }"/></svg>`, ['hex #abc']],
    ['bound class object arbitrary', 'A.vue', `<div :class="{ 'bg-[#abc]': ok }">`, ['hex #abc']],
    ['arbitrary value with underscores', 'A.vue', `<div class="shadow-[0_1px_red]">`, ['named color red']],
    ['vue style block nested', 'A.vue', `<template><div/></template><style>.a{color:#abc;& .b{color:#def}}</style>`, ['hex #abc', 'hex #def']],
    ['conditional v-bind object', 'A.vue', `<svg><circle v-bind="ok ? { fill: '#abc' } : {}"/></svg>`, ['hex #abc']],
    ['stop-color hex', 'A.vue', `<svg><stop stop-color="#abc"/></svg>`, ['hex #abc']],
    ['bound stop-color named', 'A.vue', `<svg><stop :stop-color="'red'"/></svg>`, ['named color red']],
    ['flood-color named', 'A.vue', `<svg><feFlood flood-color="red"/></svg>`, ['named color red']],
    ['v-bind nested style object', 'A.vue', `<div v-bind="{ style: { color: '#abc' } }"/>`, ['hex #abc']],
    ['v-bind style string', 'A.vue', `<div v-bind="{ style: 'color: red' }"/>`, ['named color red']],
    ['ternary branches flagged, condition skipped', 'A.vue', `<svg><circle :fill="accent === 'red' ? '#abc' : '#def'"/></svg>`, ['hex #abc', 'hex #def']],
    ['script object with hex', 'A.vue', `<template><div :style="theme"/></template><script setup>const theme = { fill: '#123456' }</script>`, ['hex #123456']],
    ['data key beside regex braces', 'A.vue', `<svg><circle v-bind="{ 'data-x': /{/g, fill: '#abc' }"/></svg>`, ['hex #abc']],
    ['color after regex with slashes', 'A.vue', `<template><div/></template><script setup>const re = /\\/\\//; const c = '#abc'</script>`, ['hex #abc']],
    ['division keeps color branches', 'A.vue', `<svg><circle :fill="w / 2 > 1 ? 'red' : 'blue'"/></svg>`, ['named color red', 'named color blue']],
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
    ['v-bind data key', 'A.vue', `<svg><circle v-bind="{ 'data-color': 'red' }"/></svg>`],
    ['v-bind data key with clean fill', 'A.vue', `<svg><circle v-bind="{ fill: 'var(--x)', 'data-accent': 'blue' }"/></svg>`],
    ['bound data attribute', 'A.vue', `<svg><circle :data-color="'red'"/></svg>`],
    ['style object named key', 'A.vue', `<div :style="{ red: '0' }">`],
    ['supports prelude', 'a.css', '@supports (color:red){.x{color:var(--x)}}'],
    ['media prelude', 'a.css', '@media (min-width:100px){.x{color:var(--x)}}'],
    ['import filename', 'a.css', '@import url(red.css);.x{}'],
    ['scss variable', 'a.css', '.x{color:$red}'],
    ['template comment', 'A.vue', `<!-- <div style="color:red"> -->`],
    ['vue style block selector', 'A.vue', `<template><div/></template><style>.red{color:var(--x)}</style>`],
    ['v-bind title string', 'A.vue', `<div v-bind="{ title: 'red' }"/>`],
    ['clip-path fragment', 'a.css', '.x{clip-path:url(#abc)}'],
    ['quoted clip-path fragment', 'a.css', '.x{clip-path:url("#abc")}'],
    ['v-bind identifier', 'A.vue', `<div v-bind="attrs"/>`],
    ['ternary condition is not paint', 'A.vue', `<svg><circle :fill="accent === 'red' ? 'currentColor' : 'none'"/></svg>`],
    ['script comment with color', 'A.vue', `<template><div/></template><script setup>// fill #abc\nconst x = 1</script>`],
    ['script identifier with color word', 'A.vue', `<template><div/></template><script setup>const preferred = true</script>`],
    ['regex contents are not paint', 'A.vue', `<svg><circle :fill="s.replace(/red|#abc/g, x)"/>`],
  ])('passes: %s', (_name, file, body) => {
    expect(checkFile(join(root, file), body)).toEqual([])
  })
})
