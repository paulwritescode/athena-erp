// The Record Type library: the list, the selection shared by Build and View,
// and the journaled save flow (destructive saves wait for review in
// MigrationDialog). Behaviour matches the pre-B2 App.vue exactly.
import { computed, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { deleteRecordType, listRecordTypes, planRecordTypeSave, saveRecordType, type SaveAnswers, type SavePlan } from '@/api'
import type { Kind, RecordType } from '@/types'
import type { DeskRoute } from './useDeskRoute'

export type RecordTypeLibrary = ReturnType<typeof useRecordTypes>

// One entry of the name-first create dialog (§20): the label the desk shows,
// the key the table takes, and for Child kinds the saved parent it belongs to.
export type NewRecordTypeInput = { name: string; description: string; kind: Kind; key: string; parent?: string }

// A write may only send the key the address names. Anything else is a stale
// or fallback object that must never save.
export function keyMatchesRoute(recordTypeName: string | null | undefined, routeKey: string): boolean {
  return !!recordTypeName && recordTypeName === routeKey
}

export function useRecordTypes(route: DeskRoute) {
  const recordTypes = ref<RecordType[]>([])
  // True once the list has loaded; stale addresses can only be judged then.
  const loaded = ref(false)
  const loadFailed = ref(false)
  const emptyRecordType: RecordType = { name: '', label: '', description: '', kind: 'master', fields: [], revision: 0 }
  const currentRecordType = computed(() => recordTypes.value.find((item) => item.name === route.selectedRecordType.value) ?? recordTypes.value[0] ?? emptyRecordType)
  // View works only with saved Record Types.
  const viewRecordTypes = computed(() => recordTypes.value.filter((item) => item.revision > 0))
  const viewRecordType = computed(() => viewRecordTypes.value.find((item) => item.name === route.selectedRecordType.value) ?? viewRecordTypes.value[0] ?? null)
  // The address names a type that is not saved (renamed, deleted, mistyped).
  // Nothing may fall back to another type: no list, no form, no breadcrumb.
  const unknownSelectedType = computed(() => route.mode.value === 'view' && route.page.value !== 'queue' && route.page.value !== 'count' && loaded.value && route.selectedRecordType.value !== '' && !viewRecordTypes.value.some((item) => item.name === route.selectedRecordType.value))

  async function load() {
    loadFailed.value = false
    try {
      recordTypes.value = await listRecordTypes()
      loaded.value = true
    } catch (err) {
      loadFailed.value = true
      toast.error(err instanceof ConnectError ? `Could not load Record Types: ${err.rawMessage}` : 'Could not load Record Types.')
    }
  }

  function clear() {
    recordTypes.value = []
    loaded.value = false
    loadFailed.value = false
  }

  // Name-first creation (§20): draft the named type with its one required Name
  // field, save it, and roll the draft back when the save fails.
  async function createRecordType(input: NewRecordTypeInput, complete?: (saved: boolean, key?: string) => void, databaseConnected: () => boolean = () => true) {
    if (!databaseConnected()) {
      toast.error('The database is not connected. This change is only in this browser session.')
      complete?.(false)
      return
    }
    const draft: RecordType = {
      name: input.key, label: input.name, description: input.description, kind: input.kind,
      class: input.kind === 'document' ? 'non_posting' : undefined, parent_record_type: input.parent,
      fields: [{ name: 'name', label: 'Name', type: 'Text', required: true }], revision: 0,
    }
    recordTypes.value.push(draft)
    route.selectedRecordType.value = draft.name
    let savedKey: string | undefined
    try {
      const result = await saveRecordType(draft, undefined)
      draft.revision = result.saved.revision
      savedKey = draft.name
      toast.success(`${draft.label} created.`)
    } catch (err) {
      recordTypes.value = recordTypes.value.filter((item) => item !== draft)
      saveError(err)
    }
    complete?.(savedKey !== undefined, savedKey)
  }

  async function removeRecordType(recordType: RecordType) {
    if (recordType.revision > 0) {
      try {
        await deleteRecordType(recordType)
      } catch (err) {
        toast.error(err instanceof ConnectError ? `Could not delete: ${err.rawMessage}` : 'Could not delete this Record Type.')
        return
      }
    }
    recordTypes.value = recordTypes.value.filter((item) => item !== recordType)
    route.selectedRecordType.value = recordTypes.value[0]?.name ?? ''
    toast.success(recordType.revision > 0 ? `${recordType.label} deleted.` : `${recordType.label} discarded.`)
  }

  function addField() {
    currentRecordType.value.fields.push({ name: `field_${currentRecordType.value.fields.length + 1}`, label: 'New field', type: 'Text', required: false })
  }

  // A save that would delete values or needs fill-in values waits here for the
  // person to review its plan in MigrationDialog.
  const pendingSave = ref<{ recordType: RecordType; plan: SavePlan; complete?: (saved: boolean) => void } | null>(null)

  function saveError(err: unknown) {
    toast.error(err instanceof ConnectError ? `Could not save: ${err.rawMessage}` : 'Could not save this Record Type.')
  }

  async function saveCurrent(complete?: (saved: boolean) => void, databaseConnected: () => boolean = () => true) {
    if (!databaseConnected()) {
      toast.error('The database is not connected. This change is only in this browser session.')
      complete?.(false)
      return
    }
    const recordType = currentRecordType.value
    if (recordType.revision > 0) {
      try {
        const plan = await planRecordTypeSave(recordType)
        if (plan.requiresConfirmation) {
          pendingSave.value = { recordType, plan, complete }
          return
        }
      } catch (err) {
        saveError(err)
        complete?.(false)
        return
      }
    }
    await commitSave(recordType, undefined, complete)
  }

  async function commitSave(recordType: RecordType, answers: SaveAnswers | undefined, complete?: (saved: boolean) => void) {
    let saved = false
    try {
      // A saved parent whose formulas read this child's fields is saved in the
      // same request, so its rollup needs no second save.
      const parent = recordType.kind === 'child' ? recordTypes.value.find((item) => item.name === recordType.parent_record_type && item.revision > 0 && item.fields.some((field) => field.formula?.includes(`doc.${recordType.name}.`))) : undefined
      const result = await saveRecordType(recordType, answers, parent)
      recordType.revision = result.saved.revision
      if (parent && result.parent) parent.revision = result.parent.revision
      toast.success(parent ? `${recordType.label} and ${parent.label} saved.` : `${recordType.label} saved.`)
      saved = true
    } catch (err) {
      saveError(err)
    }
    complete?.(saved)
  }

  function confirmPendingSave(answers: SaveAnswers) {
    const pending = pendingSave.value
    pendingSave.value = null
    if (pending) void commitSave(pending.recordType, answers, pending.complete)
  }

  function cancelPendingSave() {
    pendingSave.value?.complete?.(false)
    pendingSave.value = null
  }

  return {
    recordTypes, currentRecordType, viewRecordTypes, viewRecordType, unknownSelectedType, pendingSave, loaded, loadFailed,
    load, clear, createRecordType, removeRecordType, addField,
    saveCurrent, confirmPendingSave, cancelPendingSave,
  }
}
