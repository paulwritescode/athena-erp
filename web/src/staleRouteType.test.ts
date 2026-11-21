// Stale-route write suppression: an address naming an unknown Record Type
// must never resolve to another type, and a write may only send the key
// the address names.
import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

vi.mock('@/api', () => ({}))

import { keyMatchesRoute, useRecordTypes } from './composables/useRecordTypes'
import type { DeskRoute } from './composables/useDeskRoute'
import type { RecordType } from '@/types'

function fakeRoute() {
  return {
    mode: ref<'build' | 'view'>('view'),
    page: ref('overview'),
    selectedRecordType: ref(''),
    recordRoute: ref(''),
  } as unknown as DeskRoute
}

function savedType(name: string): RecordType {
  return { name, label: name, description: '', kind: 'master', fields: [], revision: 1 }
}

describe('stale record type', () => {
  it.each([
    ['unknown key is unknown', 'retired', true],
    ['known key resolves', 'task', false],
    ['empty selection falls back, never unknown', '', false],
  ])('%s', (_name, selected, expected) => {
    const route = fakeRoute()
    const library = useRecordTypes(route)
    library.recordTypes.value = [savedType('task'), savedType('note')]
    library.loaded.value = true
    route.selectedRecordType.value = selected
    expect(library.unknownSelectedType.value).toBe(expected)
  })

  it('stays undecided until the list loads', () => {
    const route = fakeRoute()
    const library = useRecordTypes(route)
    route.selectedRecordType.value = 'retired'
    expect(library.loaded.value).toBe(false)
    expect(library.unknownSelectedType.value).toBe(false)
  })

  it('ignores stubs and build mode', () => {
    const route = fakeRoute()
    const library = useRecordTypes(route)
    library.recordTypes.value = [savedType('task')]
    library.loaded.value = true
    route.selectedRecordType.value = 'retired'
    route.page.value = 'queue'
    expect(library.unknownSelectedType.value).toBe(false)
    route.page.value = 'overview'
    route.mode.value = 'build'
    expect(library.unknownSelectedType.value).toBe(false)
  })

  it('falls back to the first saved type only for an empty selection', () => {
    const route = fakeRoute()
    const library = useRecordTypes(route)
    library.recordTypes.value = [savedType('task'), savedType('note')]
    expect(library.viewRecordType.value?.name).toBe('task')
  })

  it.each([
    ['matching key saves', 'task', 'task', true],
    ['stale key does not save', 'task', 'retired', false],
    ['missing type does not save', null, 'task', false],
    ['empty route key does not save', 'task', '', false],
  ])('%s', (_name, name, routeKey, expected) => {
    expect(keyMatchesRoute(name, routeKey)).toBe(expected)
  })
})
