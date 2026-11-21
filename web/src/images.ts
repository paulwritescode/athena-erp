// Small image helpers for the desk. Image bytes ride data: URLs because the
// desk's Content-Security-Policy allows img-src data: but not blob:.

/** The MIME type of a Workspace image path, sniffed by the server at upload. */
export function mimeTypeForImage(path: string): string {
  if (path.endsWith('.png')) return 'image/png'
  if (path.endsWith('.webp')) return 'image/webp'
  return 'image/jpeg'
}

/** Renders bytes as base64 in chunks, so large images never overflow the stack. */
export function bytesToBase64(content: Uint8Array): string {
  let text = ''
  const step = 0x8000
  for (let at = 0; at < content.length; at += step) {
    text += String.fromCharCode(...content.subarray(at, at + step))
  }
  return btoa(text)
}

/** A data: URL showing content as the image at path. */
export function imageDataUrl(path: string, content: Uint8Array): string {
  return `data:${mimeTypeForImage(path)};base64,${bytesToBase64(content)}`
}
