// DeskTopBar is the neutral header with a hairline: a breadcrumb on the
// left only when the page is deeper than one level (an open Record), then
// search, the Build/View switch, theme, notifications and the user menu on
// the right. Below 640 px it keeps the mark, search, notifications and the
// avatar; the mode switch moves into the user menu.
<script setup lang="ts">
import { Moon, Search, Sun } from '@/icons'
import WorkspaceMark from '@/components/WorkspaceMark.vue'
import { identity } from '@/identity'
import { Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import type { SessionUser } from '@/types'
import type { ThemeMode } from '@/composables/useTheme'
import type { NotificationTarget } from '@/notifications'
import ModeSwitch from './ModeSwitch.vue'
import NotificationsPopover from './NotificationsPopover.vue'
import UserMenu from './UserMenu.vue'
import type { PreviewState } from '@/composables/usePreview'

defineProps<{ breadcrumb: { parent: string; current: string } | null; mode: 'build' | 'view'; canBuildDesk: boolean; user: SessionUser | null; isDark: boolean; themeMode: ThemeMode; themeLabel: string; workspaceName: string; preview: PreviewState; previewAllowed: boolean; previewDeniedReason: string }>()
const emit = defineEmits<{ parentClick: []; setMode: ['build' | 'view']; openPalette: []; toggleTheme: []; setTheme: [ThemeMode]; changePassword: []; signOut: []; navigateNotification: [NotificationTarget]; previewSelect: [name: string]; previewExit: []; previewRetry: [] }>()
</script>

<template>
  <header class="bg-background/95 supports-[backdrop-filter]:bg-background/60 sticky top-0 z-30 flex h-14 shrink-0 items-center gap-2 border-b px-3 backdrop-blur min-[640px]:px-4">
    <WorkspaceMark :height="24" class="min-[640px]:hidden" />
    <span class="min-w-0 flex-1 truncate text-sm font-semibold min-[640px]:hidden">{{ identity.displayName || workspaceName || 'Athena' }}</span>
    <Breadcrumb v-if="breadcrumb" class="min-w-0 flex-1 max-[639px]:hidden">
      <BreadcrumbList class="flex-nowrap">
        <BreadcrumbItem class="min-w-0 shrink">
          <BreadcrumbLink as-child>
            <button class="max-w-32 cursor-pointer truncate sm:max-w-48" @click="emit('parentClick')">{{ breadcrumb.parent }}</button>
          </BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator class="shrink-0" />
        <BreadcrumbItem class="min-w-0">
          <BreadcrumbPage class="truncate">{{ breadcrumb.current }}</BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
    <div class="ml-auto flex shrink-0 items-center gap-1 min-[640px]:gap-2">
      <Button variant="outline" class="text-muted-foreground hidden w-56 justify-start font-normal min-[1024px]:inline-flex" aria-label="Search pages and actions" @click="emit('openPalette')">
        <Search class="size-4" aria-hidden="true" />
        <span>Search...</span>
        <kbd class="bg-muted ml-auto rounded border px-1 text-caption font-medium">⌘K</kbd>
      </Button>
      <Button variant="ghost" size="icon-sm" class="min-[1024px]:hidden" aria-label="Search pages and actions" @click="emit('openPalette')">
        <Search class="size-4" aria-hidden="true" />
      </Button>
      <ModeSwitch v-if="canBuildDesk" class="hidden min-[640px]:flex" :model-value="mode" @update:model-value="(next) => emit('setMode', next)" />
      <Button variant="ghost" size="icon-sm" class="hidden min-[640px]:inline-flex" :aria-label="`Theme ${themeLabel}, switch theme`" @click="emit('toggleTheme')">
        <component :is="isDark ? Sun : Moon" class="size-4" aria-hidden="true" />
      </Button>
      <NotificationsPopover @navigate="(target) => emit('navigateNotification', target)" />
      <UserMenu v-if="user" :user="user" :is-dark="isDark" :theme-mode="themeMode" :mode="mode" :can-build-desk="canBuildDesk" :preview="preview" :preview-allowed="previewAllowed" :preview-denied-reason="previewDeniedReason" @set-mode="(next) => emit('setMode', next)" @set-theme="(next) => emit('setTheme', next)" @change-password="emit('changePassword')" @sign-out="emit('signOut')" @preview-select="(name) => emit('previewSelect', name)" @preview-exit="emit('previewExit')" @preview-retry="emit('previewRetry')" />
    </div>
  </header>
</template>
