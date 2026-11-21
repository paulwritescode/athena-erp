// Small image helpers for the desk. Image bytes ride data: URLs because the
// desk's Content-Security-Policy allows img-src data: but not blob:.
import { Code, ConnectError } from '@connectrpc/connect'
import { fileClient } from './api'
import { maxImageBytes } from './fieldRules'

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

/** The images/ extension for a picked file, or '' when it is not an image. */
export function imageExtensionFor(file: File): string {
  if (file.type === 'image/png') return '.png'
  if (file.type === 'image/jpeg') return '.jpg'
  if (file.type === 'image/webp') return '.webp'
  const name = file.name.toLowerCase()
  if (name.endsWith('.png')) return '.png'
  if (name.endsWith('.jpg') || name.endsWith('.jpeg')) return '.jpg'
  if (name.endsWith('.webp')) return '.webp'
  return ''
}

function slug(text: string): string {
  return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'image'
}

/** A fresh images/ path for an upload, unique per call. */
export function imageUploadPath(prefix: string, file: File): string {
  return `images/${slug(prefix)}-${crypto.randomUUID().slice(0, 8)}${imageExtensionFor(file)}`
}

/** Why an upload failed, in plain words. */
export function describeImageUpload(err: unknown): string {
  if (!(err instanceof ConnectError)) return 'The image could not be uploaded.'
  if (err.code === Code.PermissionDenied) return 'You do not have permission to upload images.'
  if (err.code === Code.Canceled) return 'The upload was cancelled.'
  return `The image could not be uploaded: ${err.rawMessage}`
}

/**
 * Uploads a picked file through PutImage. Returns the stored path and hash,
 * or the reason it cannot be uploaded (too big, not an image) without
 * sending anything. RPC failures throw for describeImageUpload.
 */
export async function uploadImage(prefix: string, file: File): Promise<{ path: string; hash: string } | { error: string }> {
  if (file.size > maxImageBytes) return { error: 'Images are at most 5 MB.' }
  if (!imageExtensionFor(file)) return { error: 'Use a PNG, JPEG or WebP image.' }
  const path = imageUploadPath(prefix, file)
  const res = await fileClient.putImage({ path, content: new Uint8Array(await file.arrayBuffer()), expectedHash: '' })
  return { path, hash: res.info?.hash ?? '' }
}
