// App.vue keeps only the desk shell and its mode: the sign-in page until the
// server reports a session, then the sidebar, top bar, page switch and phone
// navigation. Route, session, Record Types and theme each live in a
// composable; the page registry drives nav and the page switch.
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { Toaster } from '@/components/ui/sonner'
import ChangePasswordDialog from './components/ChangePasswordDialog.vue'
import MigrationDialog from './components/MigrationDialog.vue'
import PageSwitch from './components/PageSwitch.vue'
import AppSidebar from './components/shell/AppSidebar.vue'
import BottomNav from './components/shell/BottomNav.vue'
import CommandPalette from './components/shell/CommandPalette.vue'
import DeskTopBar from './components/shell/DeskTopBar.vue'
import { useDeskRoute } from './composables/useDeskRoute'
import { useRecordTypes } from './composables/useRecordTypes'
import { useSession } from './composables/useSession'
import { useTheme } from './composables/useTheme'
import SignInPage from './pages/SignInPage.vue'
import { visibleBuildEntries } from './pages/registry'
import type { Page } from './types'

const route = useDeskRoute()
const library = useRecordTypes(route)
route.setDefaultTypeResolver(() => library.viewRecordTypes.value[0]?.name ?? '')
const session = useSession(route, library)
const theme = useTheme()

// The sidebar is full at >=1024 and an icon rail from 640 to 1023. The
// footer toggle overrides that until the viewport crosses the breakpoint.
const isDesktop = useMediaQuery('(min-width: 1024px)')
const isTablet = useMediaQuery('(min-width: 640px) and (max-width: 1023px)')
const sidebarOverride = ref<boolean | null>(null)
const sidebarOpen = computed(() => sidebarOverride.value ?? isDesktop.value)
// On tablet an expanded rail overlays content instead of pushing it.
const sidebarOverlay = computed(() => isTablet.value && sidebarOpen.value)
watch(isDesktop, () => { sidebarOverride.value = null })

function toggleSidebar() {
  sidebarOverride.value = !(sidebarOverride.value ?? isDesktop.value)
}

const paletteOpen = ref(false)

const buildEntries = computed(() => visibleBuildEntries(session.canBuild.value, session.user.value?.systemPermissions ?? []))
// The selected Record Type while View shows Records, else null.
const activeRecordType = computed(() => (route.mode.value === 'view' && route.page.value !== 'queue' && route.page.value !== 'count' ? route.selectedRecordType.value : null))
// The top bar names nothing at depth 1 (the page h1 carries the one name);
// an open Record adds its Record Type as the way back to the list.
const breadcrumb = computed(() => {
  if (route.mode.value !== 'view' || !route.recordRoute.value || !library.viewRecordType.value || library.unknownSelectedType.value) return null
  const id = route.recordRoute.value
  return {
    parent: library.viewRecordType.value.label,
    current: id === 'new' ? 'New record' : id.length > 12 ? `${id.slice(0, 8)}…` : id,
  }
})

function go(key: Page) {
  if (key === 'queue' || key === 'count') route.openViewStub(key)
  else route.openBuildPage(key)
}

onMounted(async () => {
  theme.init()
  await session.startDesk()
})
</script>

<template>
  <SignInPage
    v-if="session.authState.value === 'signed-out'"
    :has-users="session.hasUsers.value"
    :workspace-name="session.workspaceName.value"
    :page-state="session.pageState.value"
    :note="session.signInNote.value"
    :checking="session.rechecking.value"
    @signed-in="session.signedIn"
    @recheck="session.recheck"
  />
  <SidebarProvider v-else-if="session.authState.value === 'signed-in'" :open="sidebarOpen" :class="sidebarOverlay ? 'sidebar-tablet-overlay' : undefined" @update:open="toggleSidebar">
    <AppSidebar
      :expanded="sidebarOpen"
      :overlay="sidebarOverlay"
      :mode="route.mode.value"
      :active-page="route.page.value"
      :can-build="session.canBuild.value"
      :permissions="session.user.value?.systemPermissions ?? []"
      :record-types="library.recordTypes.value"
      :view-record-types="library.viewRecordTypes.value"
      :active-record-type="activeRecordType"
      :workspace-name="session.workspaceName.value"
      @home="route.openBuildPage('overview')"
      @navigate="go"
      @record-type="route.openRecordType"
      @toggle="toggleSidebar"
    />
    <SidebarInset class="min-w-0">
      <DeskTopBar
        :breadcrumb="breadcrumb"
        :mode="route.mode.value"
        :can-build-desk="session.canBuildDesk.value"
        :user="session.user.value"
        :is-dark="theme.isDark.value"
        :workspace-name="session.workspaceName.value"
        @parent-click="route.recordRoute.value = ''"
        @set-mode="route.setMode"
        @open-palette="paletteOpen = true"
        @toggle-theme="theme.toggle"
        @change-password="session.changingPassword.value = true"
        @sign-out="session.signOut"
      />
      <PageSwitch :route="route" :session="session" :library="library" />
    </SidebarInset>
    <BottomNav
      :mode="route.mode.value"
      :build-entries="buildEntries"
      :view-record-types="library.viewRecordTypes.value"
      :active-page="route.page.value"
      :active-record-type="activeRecordType"
      @navigate="go"
      @record-type="route.openRecordType"
    />
    <CommandPalette
      v-model:open="paletteOpen"
      :build-entries="buildEntries"
      :record-types="library.viewRecordTypes.value"
      :is-dark="theme.isDark.value"
      @navigate="go"
      @record-type="route.openRecordType"
      @toggle-theme="theme.toggle"
      @sign-out="session.signOut"
    />
    <MigrationDialog
      v-if="library.pendingSave.value"
      :record-type="library.pendingSave.value.recordType"
      :plan="library.pendingSave.value.plan"
      @confirm="library.confirmPendingSave"
      @cancel="library.cancelPendingSave"
    />
    <ChangePasswordDialog v-model:open="session.changingPassword.value" />
  </SidebarProvider>
  <Toaster position="bottom-right" :theme="theme.isDark.value ? 'dark' : 'light'" />
</template>
