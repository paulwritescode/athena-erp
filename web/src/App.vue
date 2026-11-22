// App.vue keeps only the desk shell and its mode: the sign-in page until the
// server reports a session, then the sidebar, top bar, page switch and phone
// navigation. Route, session, Record Types and theme each live in a
// composable; the page registry drives nav and the page switch.
<script setup lang="ts">
import { computed, onMounted, provide, ref, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { toast } from 'vue-sonner'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { Toaster } from '@/components/ui/sonner'
import ChangePasswordDialog from './components/ChangePasswordDialog.vue'
import IdleWarningDialog from './components/IdleWarningDialog.vue'
import MigrationDialog from './components/MigrationDialog.vue'
import StepUpDialog from './components/StepUpDialog.vue'
import PageSwitch from './components/PageSwitch.vue'
import AppSidebar from './components/shell/AppSidebar.vue'
import BottomNav from './components/shell/BottomNav.vue'
import CommandPalette from './components/shell/CommandPalette.vue'
import DeskTopBar from './components/shell/DeskTopBar.vue'
import { useDeskRoute } from './composables/useDeskRoute'
import { useOnlineStatus } from './composables/useOnlineStatus'
import { usePreview } from './composables/usePreview'
import { useRecordTypes } from './composables/useRecordTypes'
import { useSession } from './composables/useSession'
import { useSwipeNav } from './composables/useSwipeNav'
import { useTheme } from './composables/useTheme'
import OfflinePage from './pages/OfflinePage.vue'
import SignInPage from './pages/SignInPage.vue'
import type { NotificationTarget } from './notifications'
import { sheetDepth } from './components/overlayPresence'
import { visibleBuildEntries } from './pages/registry'
import type { Page } from './types'

const route = useDeskRoute()
const { online } = useOnlineStatus()
const library = useRecordTypes(route)
route.setDefaultTypeResolver(() => library.viewRecordTypes.value[0]?.name ?? '')
const session = useSession(route, library)
// Role preview ("Preview as") needs the full type universe (build) and the
// Role list (access:roles); without either the control stays disabled with
// the reason. Preview lives in Build mode only: leaving Build exits it.
const preview = usePreview()
const canListRoles = computed(() => session.user.value?.systemPermissions.includes('access:roles') ?? false)
const previewAllowed = computed(() => session.canBuild.value && canListRoles.value)
const previewDeniedReason = computed(() => {
  if (!session.canBuild.value && !canListRoles.value) return 'Needs the build and access:roles permissions.'
  if (!session.canBuild.value) return 'Needs the build permission.'
  return 'Needs the access:roles permission.'
})
watch(() => session.user.value, (user) => {
  if (user?.systemPermissions.includes('access:roles')) void preview.load()
  else preview.unload()
})
watch(() => route.mode.value, (mode) => {
  if (mode !== 'build') preview.exit()
})
watch(online, (up) => { if (up) void session.recheck() })
const theme = useTheme()
// The Appearance page offers the theme beside the Workspace choices.
provide('athena-theme', theme)
useSwipeNav()
// Phones toast top-center below the header (§17); larger screens toast
// bottom-right, or beside an open sheet through the body class in style.css.
// Below 1024 px the lane beside a sheet is narrower than a toast, so a
// sheet open there toasts top-center over everything instead.
const isPhone = useMediaQuery('(max-width: 639px)')
const belowWide = useMediaQuery('(max-width: 1023px)')
const toastTop = computed(() => isPhone.value || (belowWide.value && sheetDepth.value > 0))

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

function goNotification(target: NotificationTarget) {
  if (target.kind === 'record') {
    route.openRecordType(target.recordType)
    route.recordRoute.value = target.recordId
  } else {
    route.openBuildPage(target.page)
  }
}

// Home follows the mode: the Build overview, or the first Record Type list
// in View.
function goHome() {
  if (route.mode.value === 'view') {
    const first = library.viewRecordTypes.value[0]
    route.openRecordType(first?.name ?? '')
  } else {
    route.openBuildPage('overview')
  }
}

// The offline page replaces the desk while the browser has no connection;
// desk state lives in this setup scope, so it survives the swap. Coming
// back refreshes the desk, in case the server is down too.
function retryOnline() {
  if (!navigator.onLine) {
    toast.info('Still offline. Check your connection, then try again.')
    return
  }
  void session.recheck()
}

onMounted(async () => {
  theme.init()
  await session.startDesk()
})
</script>

<template>
  <OfflinePage v-if="!online" @retry="retryOnline" />
  <SignInPage
    v-else-if="session.authState.value === 'signed-out'"
    :has-users="session.hasUsers.value"
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
      :username="session.user.value?.username ?? ''"
      :preview="preview"
      :preview-allowed="previewAllowed"
      :preview-denied-reason="previewDeniedReason"
      @home="goHome"
      @navigate="go"
      @record-type="route.openRecordType"
      @toggle="toggleSidebar"
      @preview-select="preview.enter"
      @preview-exit="preview.exit"
      @preview-retry="preview.load"
    />
    <SidebarInset class="min-w-0">
      <DeskTopBar
        :breadcrumb="breadcrumb"
        :mode="route.mode.value"
        :can-build-desk="session.canBuildDesk.value"
        :user="session.user.value"
        :is-dark="theme.isDark.value"
        :theme-mode="theme.mode.value"
        :theme-label="theme.modeLabel.value"
        :workspace-name="session.workspaceName.value"
        :preview="preview"
        :preview-allowed="previewAllowed"
        :preview-denied-reason="previewDeniedReason"
        @parent-click="route.recordRoute.value = ''"
        @set-mode="route.setMode"
        @open-palette="paletteOpen = true"
        @toggle-theme="theme.toggle"
        @set-theme="theme.setMode"
        @navigate-notification="goNotification"
        @change-password="session.changingPassword.value = true"
        @sign-out="session.signOut"
        @preview-select="preview.enter"
        @preview-exit="preview.exit"
        @preview-retry="preview.load"
      />
      <PageSwitch :route="route" :session="session" :library="library" :preview="preview" />
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
      :mode="route.mode.value"
      @navigate="go"
      @record-type="route.openRecordType"
      @toggle-theme="theme.toggle"
      @sign-out="session.signOut"
    />
    <MigrationDialog
      :record-type="library.pendingSave.value?.recordType ?? library.currentRecordType.value"
      :plan="library.pendingSave.value?.plan ?? null"
      @confirm="library.confirmPendingSave"
      @cancel="library.cancelPendingSave"
    />
    <ChangePasswordDialog v-model:open="session.changingPassword.value" />
    <StepUpDialog />
    <IdleWarningDialog />
  </SidebarProvider>
  <Toaster
    :position="toastTop ? 'top-center' : 'bottom-right'"
    :offset="toastTop ? { top: 'calc(var(--space-16) + var(--space-2))' } : undefined"
    :mobile-offset="isPhone ? { top: 'calc(var(--space-16) + var(--space-2))' } : undefined"
    :theme="theme.isDark.value ? 'dark' : 'light'"
  />
</template>
