// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { Code, ConnectError } from '@connectrpc/connect'
import { bytesToBase64, describeImageUpload, imageDataUrl, imageExtensionFor, imageUploadPath, mimeTypeForImage } from './images'

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

describe('image uploads', () => {
  it('reads the extension from the type, then the name', () => {
    expect(imageExtensionFor(new File(['x'], 'a.png', { type: 'image/png' }))).toBe('.png')
    expect(imageExtensionFor(new File(['x'], 'a.jpg', { type: 'image/jpeg' }))).toBe('.jpg')
    expect(imageExtensionFor(new File(['x'], 'a.webp', { type: '' }))).toBe('.webp')
    expect(imageExtensionFor(new File(['x'], 'a.txt', { type: 'text/plain' }))).toBe('')
  })

  it('names a fresh path per upload', () => {
    const file = new File(['x'], 'a.png', { type: 'image/png' })
    const first = imageUploadPath('item-photo', file)
    const second = imageUploadPath('item-photo', file)
    expect(first).toMatch(/^images\/item-photo-[0-9a-f]{8}\.png$/)
    expect(second).not.toBe(first)
    expect(imageUploadPath('Weird Name!', file)).toMatch(/^images\/weird-name-[0-9a-f]{8}\.png$/)
  })

  it('explains upload failures in plain words', () => {
    expect(describeImageUpload(new ConnectError('no', Code.PermissionDenied))).toContain('permission')
    expect(describeImageUpload(new ConnectError('cancelled', Code.Canceled))).toContain('cancelled')
    expect(describeImageUpload(new ConnectError('bad pixels', Code.InvalidArgument))).toContain('bad pixels')
    expect(describeImageUpload(new Error('boom'))).toBe('The image could not be uploaded.')
  })
})
