// The signed-in user's menu: identity, theme, password, sign out. Below 640
// px the Build/View switch lives here because the header has no room for it.
// Below 1024 px the menu is a bottom drawer (§13.2).
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { Check, ChevronDown, Moon, Sun } from '@/icons'
import BottomDrawer from '@/components/BottomDrawer.vue'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { SessionUser } from '@/types'

const props = defineProps<{ user: SessionUser; isDark: boolean; mode: 'build' | 'view'; canBuildDesk: boolean }>()
const emit = defineEmits<{ setMode: ['build' | 'view']; toggleTheme: []; changePassword: []; signOut: [] }>()

const name = computed(() => props.user.displayName || props.user.username)
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
        <small class="text-muted-foreground block truncate font-normal">{{ user.owner ? 'Instance owner' : user.username }}</small>
      </DropdownMenuLabel>
      <DropdownMenuSeparator />
      <DropdownMenuItem @select="emit('toggleTheme')">
        <component :is="isDark ? Sun : Moon" class="size-4" aria-hidden="true" />
        <span>{{ isDark ? 'Light theme' : 'Dark theme' }}</span>
      </DropdownMenuItem>
      <DropdownMenuItem @select="emit('changePassword')">Change password</DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem @select="emit('signOut')">Sign out</DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
  <template v-else>
    <Button variant="ghost" size="sm" class="gap-2 px-2" :aria-label="`Signed in as ${name}`" @click="open = true">
      <Avatar size="sm">
        <AvatarFallback>{{ initials }}</AvatarFallback>
      </Avatar>
    </Button>
    <BottomDrawer v-model:open="open" :title="name" :description="user.owner ? 'Instance owner' : user.username">
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
        <button type="button" role="menuitem" @click="pick(() => emit('toggleTheme'))">
          <component :is="isDark ? Sun : Moon" class="size-4" aria-hidden="true" />
          <span>{{ isDark ? 'Light theme' : 'Dark theme' }}</span>
        </button>
        <button type="button" role="menuitem" @click="pick(() => emit('changePassword'))">
          <span>Change password</span>
        </button>
        <button type="button" role="menuitem" @click="pick(() => emit('signOut'))">
          <span>Sign out</span>
        </button>
      </div>
    </BottomDrawer>
  </template>
</template>
