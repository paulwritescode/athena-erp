// Mirrors CleanPath in internal/files/path.go so the Files page reports a bad
// path before the request is sent. The server still checks every call. The
// shared table internal/files/testdata/paths.json runs in both test suites.
export const fileFolders = ['generated', 'packs', 'record_types', 'rules']
export const fileExtensions = ['.go', '.json', '.md', '.star', '.txt', '.yaml', '.yml']
export const maxFileBytes = 1 << 20

const maxPathBytes = 240
const maxSegmentBytes = 100
const maxDepth = 8
const segmentPattern = /^[a-z0-9_][a-z0-9_.-]*$/
const encoder = new TextEncoder()

/** Returns why a path cannot be managed, or null when it can. */
export function validateFilePath(path: string): string | null {
  if (path === '') return 'Enter a path.'
  if (encoder.encode(path).length > maxPathBytes) return `The path is longer than ${maxPathBytes} bytes.`
  if (path.startsWith('/')) return 'Use a path relative to the Workspace, not an absolute one.'
  if (path.includes('\\')) return 'Use / as the separator.'
  const segments = path.split('/')
  if (segments.length > maxDepth) return `The path is deeper than ${maxDepth} folders.`
  for (const segment of segments) {
    if (segment === '') return 'The path has an empty segment.'
    if (segment === '.' || segment === '..') return 'The path may not use . or ..'
    if (segment.startsWith('.')) return 'Names starting with a dot are not managed.'
    if (encoder.encode(segment).length > maxSegmentBytes) return `A name is longer than ${maxSegmentBytes} bytes.`
    if (!segmentPattern.test(segment) || segment.endsWith('.')) return 'Use lowercase letters, digits, _, - and . only.'
  }
  if (segments.length < 2) return `Put the file in a folder: ${fileFolders.join(', ')}.`
  if (!fileFolders.includes(segments[0] ?? '')) return `Use one of the folders ${fileFolders.join(', ')}.`
  const last = segments[segments.length - 1] ?? ''
  const extension = last.slice(last.lastIndexOf('.'))
  if (!fileExtensions.includes(extension)) return `The file type is not allowed. Use ${fileExtensions.join(' ')}.`
  return null
}

/** True when text is within the size limit once encoded as UTF-8. */
export function withinFileLimit(text: string): boolean {
  return encoder.encode(text).length <= maxFileBytes
}
