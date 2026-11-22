// Role preview ("Preview as"): a read-only look at View as one Role sees
// it (B8, playbook §8 "Developer entry": a simulation of another Role is a
// read-only explanation). It never shows Record contents: only which lists
// the Role's Grants reach, with the allowed actions per list. Role Grants
// come from ListRoles; the evaluation mirrors the server's whole-Grant
// check (Principal.Can/Level) over that one Role.
import { computed, ref } from 'vue'
import { authClient } from '@/api'
import type { RecordType } from '@/types'

export type PreviewGrant = { recordTypeKey: string; actions: string[]; permissionLevel: number }
export type PreviewRole = { name: string; grants: PreviewGrant[] }
export type PreviewType = { name: string; label: string; actions: string[]; hiddenFields: number }

// Canonical Grant action order, as the server lists it.
const actionOrder = ['read', 'create', 'write', 'delete', 'submit', 'cancel', 'amend', 'audit']

// roleView computes the View navigation a Role sees: the saved Record Types
// some Grant lets it read, each with the union of the covering Grants'
// actions and the count of fields above the reached level. Conditions and
// Scopes narrow rows, never the list itself, exactly like Can().
export function roleView(role: PreviewRole, types: Pick<RecordType, 'name' | 'label' | 'fields'>[]): PreviewType[] {
  const out: PreviewType[] = []
  for (const type of types) {
    const covering = role.grants.filter((grant) => grant.recordTypeKey === '*' || grant.recordTypeKey === type.name)
    if (!covering.some((grant) => grant.actions.includes('read'))) continue
    const actions = actionOrder.filter((action) => covering.some((grant) => grant.actions.includes(action)))
    const levels = covering.filter((grant) => grant.actions.length > 0).map((grant) => grant.permissionLevel)
    const maxLevel = levels.length > 0 ? Math.max(...levels) : 0
    const hiddenFields = type.fields.filter((field) => (field.permission_level ?? 0) > maxLevel).length
    out.push({ name: type.name, label: type.label, actions, hiddenFields })
  }
  return out
}

export type PreviewState = ReturnType<typeof usePreview>

export function usePreview() {
  const roles = ref<PreviewRole[]>([])
  const rolesState = ref<'idle' | 'loading' | 'ready' | 'failed'>('idle')
  const previewName = ref('')
  const previewing = computed(() => previewName.value !== '')
  const role = computed(() => roles.value.find((candidate) => candidate.name === previewName.value) ?? null)

  async function load() {
    if (rolesState.value === 'loading') return
    rolesState.value = 'loading'
    try {
      const res = await authClient.listRoles({})
      roles.value = res.roles.map((item) => ({
        name: item.name,
        grants: (item.grants ?? []).map((grant) => ({
          recordTypeKey: grant.recordTypeKey,
          actions: [...grant.actions],
          permissionLevel: grant.permissionLevel,
        })),
      }))
      rolesState.value = 'ready'
      // A previewed Role that vanished ends the preview instead of
      // explaining stale Grants.
      if (previewName.value && !role.value) previewName.value = ''
    } catch {
      rolesState.value = 'failed'
    }
  }

  function unload() {
    roles.value = []
    rolesState.value = 'idle'
    previewName.value = ''
  }

  function enter(name: string) {
    previewName.value = name
  }

  function exit() {
    previewName.value = ''
  }

  return { roles, rolesState, previewName, previewing, role, load, unload, enter, exit }
}
