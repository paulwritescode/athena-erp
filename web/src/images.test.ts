import { describe, expect, it } from 'vitest'
import { bytesToBase64, imageDataUrl, mimeTypeForImage } from './images'

describe('image data URLs', () => {
  it('maps image paths to MIME types', () => {
    expect(mimeTypeForImage('images/a.png')).toBe('image/png')
    expect(mimeTypeForImage('images/a.webp')).toBe('image/webp')
    expect(mimeTypeForImage('images/a.jpg')).toBe('image/jpeg')
    expect(mimeTypeForImage('images/a.jpeg')).toBe('image/jpeg')
  })

  it('encodes bytes as base64 without stack overflow on large input', () => {
    const bytes = new Uint8Array(200_000)
    for (let i = 0; i < bytes.length; i++) bytes[i] = i % 251
    const text = bytesToBase64(bytes)
    expect(text.length).toBeGreaterThan(0)
    expect(atob(text.slice(0, 8))).toBe('\x00\x01\x02\x03\x04\x05')
    expect(imageDataUrl('images/a.png', new Uint8Array([137, 80, 78, 71]))).toBe('data:image/png;base64,iVBORw==')
  })
})
