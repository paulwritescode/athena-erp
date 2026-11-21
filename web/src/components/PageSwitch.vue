// PageSwitch renders the current desk page from the registry: Build pages
// and the View stubs load lazily, View Records render directly. A page the
// user must not see (reached directly by address) shows PermissionDenied; a
// page that fails to load shows ErrorState with a retry.
<script setup lang="ts">
import { computed, defineAsyncComponent, onErrorCaptured, ref, watch } from 'vue'
import { FileX2 } from '@/icons'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import PermissionDenied from '@/components/PermissionDenied.vue'
import ActionButton from '@/components/ActionButton.vue'
import RecordsPage from '@/pages/RecordsPage.vue'
import { isEntryVisible, pageEntry, visibleBuildKeys, type PageContext } from '@/pages/registry'
import type { DeskRoute } from '@/composables/useDeskRoute'
import type { RecordTypeLibrary } from '@/composables/useRecordTypes'
import type { SessionState } from '@/composables/useSession'
import type { Page, RecordType } from '@/types'

const props = defineProps<{ route: DeskRoute; session: SessionState; library: RecordTypeLibrary }>()
const { route, session, library } = props

const showRecords = computed(() => route.mode.value === 'view' && route.page.value !== 'queue' && route.page.value !== 'count')
// A named type that is not saved (renamed, deleted, mistyped) never falls
// back to another type's list or form.
const unknownType = computed(() => library.unknownSelectedType.value)
const typesPending = computed(() => showRecords.value && !library.loaded.value && !library.loadFailed.value)

function goViewHome() {
  const first = library.viewRecordTypes.value[0]
  route.openRecordType(first?.name ?? '')
}
// The list falls back to the first saved type; the address names it so a
// refresh or a shared link opens the same list, record included.
watch([showRecords, () => library.viewRecordType.value?.name], ([showing, fallback]) => {
  if (showing && fallback) route.resolveSelected(fallback)
}, { immediate: true })
const entry = computed(() => {
  if (showRecords.value) return undefined
  return pageEntry(route.page.value)
})

// A valid address with no registry entry (a page landing later in the batch)
// falls back to Overview instead of rendering nothing.
watch(entry, (next) => {
  if (!showRecords.value && !next) route.page.value = 'overview'
}, { immediate: true })

const denied = computed(() => {
  const current = entry.value
  if (!current) return false
  return !isEntryVisible(current, session.canBuild.value, session.user.value?.systemPermissions ?? [])
})

const deniedTitle = computed(() => entry.value?.title ?? 'this page')

const ctx = computed<PageContext>(() => ({
  recordTypes: library.recordTypes.value,
  currentRecordType: library.currentRecordType.value,
  databaseState: session.databaseState.value,
  apiState: session.apiState.value,
  permissions: session.user.value?.systemPermissions ?? [],
  username: session.user.value?.username ?? '',
  canWriteRules: session.canWriteRules.value,
  canWriteFiles: session.canWriteFiles.value,
  canWriteLedger: session.canWriteLedger.value,
}))

const entryProps = computed<Record<string, unknown>>(() => entry.value?.props?.(ctx.value) ?? {})
const asyncPage = computed(() => (entry.value ? defineAsyncComponent(entry.value.load) : null))

const loadError = ref(false)
const retryKey = ref(0)
watch([() => route.mode.value, () => route.page.value], () => { loadError.value = false })

onErrorCaptured(() => {
  loadError.value = true
  return false
})

function retry() {
  loadError.value = false
  retryKey.value += 1
}

function goHome() {
  if (session.canBuildDesk.value) {
    const first = visibleBuildKeys(session.canBuild.value, session.user.value?.systemPermissions ?? [])[0] ?? 'overview'
    route.openBuildPage(first)
  } else {
    route.mode.value = 'view'
    route.page.value = 'overview'
    route.recordRoute.value = ''
  }
}

// Every event a registry page may emit, wired to desk state. Pages only emit
// their own; the rest stay quiet.
const listeners = {
  navigate: (page: Page) => route.openBuildPage(page),
  createRecordType: () => {
    route.openBuildPage('record-types')
    library.addRecordType()
  },
  select: (name: string) => { route.selectedRecordType.value = name },
  addRecordType: () => library.addRecordType(),
  addField: () => library.addField(),
  rename: (from: string, to: string) => library.renameRecordType(from, to),
  delete: (recordType: RecordType) => { void library.removeRecordType(recordType) },
  save: (complete: (saved: boolean) => void) => { void library.saveCurrent(complete, () => session.databaseState.value === 'Connected') },
  preview: () => route.setMode('view'),
  openRecordTypes: () => route.openBuildPage('record-types'),
  back: () => route.setMode('build'),
  goHome: () => goHome(),
}
</script>

<template>
  <div data-page-main class="min-w-0 flex-1 p-4 pb-20 sm:p-6 lg:p-8">
    <EmptyState
      v-if="unknownType"
      :icon="FileX2"
      title="This list no longer exists"
      body="The Record Type was renamed or deleted, or the address is mistyped."
    >
      <template #action>
        <ActionButton kind="primary" label="Back to Records" icon="arrow-left" @press="goViewHome" />
      </template>
    </EmptyState>
    <LoadingSkeleton v-else-if="typesPending" />
    <RecordsPage
      v-else-if="showRecords"
      v-model:record-id="route.recordRoute.value"
      :record-type="library.viewRecordType.value"
      :record-type-key="route.selectedRecordType.value"
      :record-types="library.viewRecordTypes.value"
      @back="route.setMode('build')"
    />
    <PermissionDenied v-else-if="denied" :page-title="deniedTitle" @go-home="goHome" />
    <ErrorState v-else-if="loadError" @retry="retry" />
    <Suspense v-else>
      <template #default>
        <component :is="asyncPage" :key="`${route.page.value}:${retryKey}`" v-bind="entryProps" v-on="listeners" />
      </template>
      <template #fallback>
        <LoadingSkeleton />
      </template>
    </Suspense>
  </div>
</template>
