import { describe, expect, it, vi } from 'vitest'
import { roleView, usePreview } from './composables/usePreview'
import { authClient } from '@/api'

vi.mock('@/api', () => ({
  authClient: { listRoles: vi.fn() },
}))

function field(name: string, permission_level = 0) {
  return { name, label: name, type: 'Text', required: false, permission_level }
}

describe('roleView', () => {
  const types = [
    { name: 'invoice', label: 'Invoice', fields: [field('title'), field('cost', 5)] },
    { name: 'note', label: 'Note', fields: [field('title')] },
  ]

  it('shows the readable lists with the union of the covering actions', () => {
    const role = {
      name: 'Clerk',
      grants: [
        { recordTypeKey: 'invoice', actions: ['read', 'create'], permissionLevel: 9 },
        { recordTypeKey: 'note', actions: ['write'], permissionLevel: 9 },
      ],
    }
    expect(roleView(role, types)).toEqual([{ name: 'invoice', label: 'Invoice', actions: ['read', 'create'], hiddenFields: 0 }])
  })

  it('lets a *-grant reach every list and counts hidden fields', () => {
    const role = { name: 'Reader', grants: [{ recordTypeKey: '*', actions: ['read'], permissionLevel: 2 }] }
    expect(roleView(role, types)).toEqual([
      { name: 'invoice', label: 'Invoice', actions: ['read'], hiddenFields: 1 },
      { name: 'note', label: 'Note', actions: ['read'], hiddenFields: 0 },
    ])
  })

  it('orders actions canonically across overlapping grants', () => {
    const role = {
      name: 'Clerk',
      grants: [
        { recordTypeKey: '*', actions: ['audit'], permissionLevel: 0 },
        { recordTypeKey: 'note', actions: ['delete', 'read'], permissionLevel: 0 },
      ],
    }
    expect(roleView(role, types)).toEqual([{ name: 'note', label: 'Note', actions: ['read', 'delete', 'audit'], hiddenFields: 0 }])
  })

  it('shows nothing when no Grant reads', () => {
    const role = { name: 'Empty', grants: [{ recordTypeKey: '*', actions: ['create'], permissionLevel: 0 }] }
    expect(roleView(role, types)).toEqual([])
  })
})

describe('usePreview', () => {
  it('loads Roles and ends a preview whose Role vanished', async () => {
    vi.mocked(authClient.listRoles).mockResolvedValue({
      roles: [{ name: 'Clerk', grants: [{ recordTypeKey: '*', actions: ['read'], permissionLevel: 0 }] }],
    } as never)
    const preview = usePreview()
    await preview.load()
    expect(preview.rolesState.value).toBe('ready')
    expect(preview.roles.value.map((role) => role.name)).toEqual(['Clerk'])
    preview.enter('Clerk')
    expect(preview.previewing.value).toBe(true)
    expect(preview.role.value?.name).toBe('Clerk')
    preview.exit()
    expect(preview.previewing.value).toBe(false)
    preview.enter('Clerk')
    vi.mocked(authClient.listRoles).mockResolvedValue({ roles: [] } as never)
    await preview.load()
    expect(preview.previewing.value).toBe(false)
  })

  it('reports a failed load', async () => {
    vi.mocked(authClient.listRoles).mockRejectedValue(new Error('down'))
    const preview = usePreview()
    await preview.load()
    expect(preview.rolesState.value).toBe('failed')
  })
})
