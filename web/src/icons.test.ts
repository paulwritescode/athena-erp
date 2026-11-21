import { describe, expect, it } from 'vitest'
import { FileText, icon, iconNames, type IconName } from './icons'

describe('icon registry', () => {
  it('resolves every registered name to a component', () => {
    for (const name of iconNames) {
      expect(icon(name), name).toBeTruthy()
    }
  })

  it('covers the section 5 table', () => {
    const section5: IconName[] = [
      'house', 'layout-template', 'package', 'scroll-text', 'folder', 'users', 'book-open',
      'chart-column', 'list-checks', 'settings', 'search', 'bell', 'plus', 'pencil',
      'trash-2', 'ellipsis', 'list-filter', 'arrow-up-down', 'x', 'arrow-left',
      'circle-help', 'circle-check', 'triangle-alert', 'circle-alert', 'info', 'lock',
      'wifi-off', 'sun', 'moon', 'log-out',
    ]
    expect(section5).toHaveLength(30)
    for (const name of section5) {
      expect(iconNames, name).toContain(name)
    }
  })

  it('covers the section 14 action icons', () => {
    const section14: IconName[] = [
      'plus', 'save', 'pencil', 'send', 'key-round', 'eye', 'copy',
      'pause', 'log-out', 'undo-2', 'trash-2', 'user-x', 'play', 'rotate-ccw',
    ]
    expect(section14).toHaveLength(14)
    for (const name of section14) {
      expect(iconNames, name).toContain(name)
    }
  })

  it('falls back to FileText for unknown names', () => {
    expect(icon('no-such-icon')).toBe(FileText)
    expect(icon('')).toBe(FileText)
  })

  it('uses kebab-case names only', () => {
    for (const name of iconNames) {
      expect(name, name).toMatch(/^[a-z0-9]+(-[a-z0-9]+)*$/)
    }
  })
})
