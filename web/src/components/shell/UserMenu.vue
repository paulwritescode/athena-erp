// The signed-in user's menu: identity, theme, password, sign out. Below 640
// px the Build/View switch lives here because the header has no room for it.
// Below 1024 px the menu is a bottom drawer (§13.2).
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { Check, ChevronDown, Monitor, Moon, Sun } from '@/icons'
import BottomDrawer from '@/components/BottomDrawer.vue'
import { hasLogo, identity } from '@/identity'
import { gloss } from '@/copy/glossary'
import { isRootUsername } from '@/rootUser'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { PreviewState } from '@/composables/usePreview'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { SessionUser } from '@/types'
import type { ThemeMode } from '@/composables/useTheme'

const props = defineProps<{
  user: SessionUser
  isDark: boolean
  themeMode: ThemeMode
  mode: 'build' | 'view'
  canBuildDesk: boolean
  preview: PreviewState
  previewAllowed: boolean
  previewDeniedReason: string
}>()
const emit = defineEmits<{
  setMode: ['build' | 'view']
  setTheme: [ThemeMode]
  changePassword: []
  signOut: []
  previewSelect: [name: string]
  previewExit: []
  previewRetry: []
}>()

function pickedPreview(value: unknown) {
  if (typeof value === 'string' && value) emit('previewSelect', value)
}

const themeChoices: { mode: ThemeMode; label: string }[] = [
  { mode: 'light', label: 'Light' },
  { mode: 'dark', label: 'Dark' },
  { mode: 'system', label: 'System' },
]

const name = computed(() => props.user.displayName || props.user.username)
const caption = computed(() => (isRootUsername(props.user.username) ? gloss('rootAdministrator').label : props.user.owner ? 'Instance owner' : props.user.username))
const initials = computed(() => name.value.trim().split(/\s+/).map((part) => part[0]).slice(0, 2).join('').toUpperCase() || '?')
const isDesktop = useMediaQuery('(min-width: 1024px)')
const open = ref(false)

function pick(action: () => void) {
  open.value = false
  action()
}
</script>

<template>
  <DropdownMenu v-if="isDesktop">
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" size="sm" class="gap-2 px-2" :aria-label="`Signed in as ${name}`">
        <Avatar size="sm">
          <AvatarFallback>{{ initials }}</AvatarFallback>
        </Avatar>
        <span class="hidden max-w-28 truncate min-[1024px]:inline">{{ name }}</span>
        <ChevronDown class="text-muted-foreground hidden size-3.5 min-[1024px]:inline" aria-hidden="true" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" class="w-56">
      <DropdownMenuLabel>
        <span class="block truncate font-medium">{{ name }}</span>
        <small class="text-muted-foreground block truncate font-normal">{{ caption }}</small>
      </DropdownMenuLabel>
      <DropdownMenuSeparator />
      <DropdownMenuLabel class="text-xs font-normal">Theme</DropdownMenuLabel>
      <DropdownMenuItem v-for="choice in themeChoices" :key="choice.mode" @select="emit('setTheme', choice.mode)">
        <component :is="choice.mode === 'light' ? Sun : choice.mode === 'dark' ? Moon : Monitor" class="size-4" aria-hidden="true" />
        <span class="flex-1">{{ choice.label }}</span>
        <Check v-if="themeMode === choice.mode" class="size-4" aria-hidden="true" />
      </DropdownMenuItem>
      <DropdownMenuItem @select="emit('changePassword')">Change password</DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem @select="emit('signOut')">Sign out</DropdownMenuItem>
      <template v-if="hasLogo(identity)">
        <DropdownMenuSeparator />
        <DropdownMenuLabel class="text-muted-foreground text-xs font-normal">Powered by Athena</DropdownMenuLabel>
      </template>
    </DropdownMenuContent>
  </DropdownMenu>
  <template v-else>
    <Button variant="ghost" size="sm" class="gap-2 px-2" :aria-label="`Signed in as ${name}`" @click="open = true">
      <Avatar size="sm">
        <AvatarFallback>{{ initials }}</AvatarFallback>
      </Avatar>
    </Button>
    <BottomDrawer v-model:open="open" :title="name" :description="caption">
      <!-- Phones and tablets preview Roles here: the sidebar group needs labels. -->
      <div v-if="mode === 'build'" class="preview-drawer-block">
        <p class="text-muted-foreground px-3 text-caption">Preview as</p>
        <p v-if="!previewAllowed" class="text-muted-foreground px-3 py-1 text-caption">{{ previewDeniedReason }}</p>
        <template v-else-if="preview.rolesState.value === 'failed'">
          <p class="text-destructive px-3 py-1 text-caption">Could not load Roles.</p>
          <div class="drawer-menu" role="menu" aria-label="Preview">
            <button type="button" role="menuitem" @click="emit('previewRetry')">
              <span>Retry</span>
            </button>
          </div>
        </template>
        <Select
          v-else
          :model-value="preview.previewName.value === '' ? undefined : preview.previewName.value"
          :disabled="preview.rolesState.value !== 'ready' || preview.roles.value.length === 0"
          @update:model-value="pickedPreview"
        >
          <SelectTrigger class="w-full" aria-label="Preview as a Role">
            <SelectValue :placeholder="preview.rolesState.value === 'loading' ? 'Loading Roles…' : preview.roles.value.length === 0 ? 'No Roles yet' : 'Choose a Role…'" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="role in preview.roles.value" :key="role.name" :value="role.name">{{ role.name }}</SelectItem>
          </SelectContent>
        </Select>
        <div v-if="preview.previewName.value" class="drawer-menu" role="menu" aria-label="Preview">
          <button type="button" role="menuitem" @click="pick(() => emit('previewExit'))">
            <span>Exit preview</span>
          </button>
        </div>
      </div>
      <div class="drawer-menu" role="menu" :aria-label="`Signed in as ${name}`">
        <template v-if="canBuildDesk">
          <button type="button" role="menuitemradio" :aria-checked="mode === 'build'" class="min-[640px]:hidden" @click="pick(() => emit('setMode', 'build'))">
            <span class="flex-1">Build mode</span>
            <Check v-if="mode === 'build'" class="size-4" aria-hidden="true" />
          </button>
          <button type="button" role="menuitemradio" :aria-checked="mode === 'view'" class="min-[640px]:hidden" @click="pick(() => emit('setMode', 'view'))">
            <span class="flex-1">View mode</span>
            <Check v-if="mode === 'view'" class="size-4" aria-hidden="true" />
          </button>
        </template>
        <button
          v-for="choice in themeChoices"
          :key="choice.mode"
          type="button"
          role="menuitemradio"
          :aria-checked="themeMode === choice.mode"
          @click="pick(() => emit('setTheme', choice.mode))"
        >
          <component :is="choice.mode === 'light' ? Sun : choice.mode === 'dark' ? Moon : Monitor" class="size-4" aria-hidden="true" />
          <span class="flex-1">{{ choice.label }} theme</span>
          <Check v-if="themeMode === choice.mode" class="size-4" aria-hidden="true" />
        </button>
        <button type="button" role="menuitem" @click="pick(() => emit('changePassword'))">
          <span>Change password</span>
        </button>
        <button type="button" role="menuitem" @click="pick(() => emit('signOut'))">
          <span>Sign out</span>
        </button>
        <p v-if="hasLogo(identity)" class="text-muted-foreground px-4 pt-2 text-xs">Powered by Athena</p>
      </div>
    </BottomDrawer>
  </template>
</template>

<style scoped>
.preview-drawer-block {
  display: grid;
  gap: var(--space-1);
  padding-bottom: var(--space-3);
  margin-bottom: var(--space-2);
  border-bottom: 1px solid var(--border);
}
</style>
