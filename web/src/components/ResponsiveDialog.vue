// ResponsiveDialog is the one dialog shell: a centered dialog at 1024 px and
// wider, a vaul bottom drawer below. Dismissing a dirty form asks first
// (one nested layer); setting dismissible false holds it shut while busy.
<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import ActionButton from '@/components/ActionButton.vue'
import BottomDrawer from '@/components/BottomDrawer.vue'
import { guardNext } from './dismissGuard'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'

const props = withDefaults(defineProps<{
  title: string
  description?: string
  wide?: boolean
  dirty?: boolean
  dirtyTitle?: string
  dirtyBody?: string
  dismissible?: boolean
}>(), {
  wide: false,
  dirty: false,
  dirtyTitle: 'Discard your edits?',
  dirtyBody: 'Changes that are not saved go away.',
  dismissible: true,
})
const open = defineModel<boolean>('open', { required: true })
const isDesktop = useMediaQuery('(min-width: 1024px)')
const confirming = ref(false)

function onOpenChange(next: boolean) {
  if (next) {
    open.value = true
    return
  }
  if (!props.dismissible) return
  const decision = guardNext(props.dirty, 'requestDismiss')
  confirming.value = decision.confirming
  if (decision.close) open.value = false
}

function onGuard(action: 'discard' | 'keep') {
  const decision = guardNext(props.dirty, action)
  confirming.value = decision.confirming
  if (decision.close) {
    open.value = false
    return
  }
  // A fast flick can outrun the controlled close and hide the drawer before
  // the guard opens. If vaul already hid the main drawer, bounce the model
  // to resync it.
  if (action === 'keep' && !isDesktop.value) {
    const mainVisible = [...document.querySelectorAll('[data-vaul-drawer]')].some((el) =>
      (el.textContent ?? '').includes(props.title),
    )
    if (!mainVisible) {
      open.value = false
      void nextTick(() => {
        open.value = true
      })
    }
  }
}

function guardForce(event: Event) {
  if (!props.dismissible) {
    event.preventDefault()
    return
  }
  // Popovers portal outside the dialog: interacting with one (a combobox
  // inside this dialog) must not dismiss it.
  const target = event.target as HTMLElement | null
  if (target?.closest?.('[data-slot="popover-content"]')) event.preventDefault()
}

// The dialog is modal, so focus can only leave it for our own portalled
// popovers (combobox search inputs autofocus on open): focus-outside never
// dismisses. Pointer dismissal still applies outside popovers.
function guardFocus(event: Event) {
  event.preventDefault()
}
</script>

<template>
  <Dialog v-if="isDesktop" :open="open" @update:open="onOpenChange">
    <DialogContent
      :show-close-button="dismissible"
      :class="wide ? 'sm:max-w-lg' : undefined"
      @escape-key-down="guardForce"
      @pointer-down-outside="guardForce"
      @focus-outside="guardFocus"
      @interact-outside="guardForce"
    >
      <DialogHeader>
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription v-if="description">{{ description }}</DialogDescription>
      </DialogHeader>
      <div v-if="$slots.default" class="overlay-stack">
        <slot />
      </div>
      <DialogFooter v-if="$slots.footer">
        <slot name="footer" />
      </DialogFooter>
    </DialogContent>
  </Dialog>
  <BottomDrawer v-else :open="open" :title="title" :description="description" :dismissible="dismissible" :dirty="dirty" @update:open="onOpenChange">
    <slot />
    <template v-if="$slots.footer" #footer>
      <slot name="footer" />
    </template>
  </BottomDrawer>
  <BottomDrawer v-if="!isDesktop" :open="confirming" :title="dirtyTitle" :description="dirtyBody" @update:open="(next: boolean) => { confirming = next }">
    <template #footer>
      <ActionButton kind="secondary" label="Keep editing" icon="pencil" @press="onGuard('keep')" />
      <ActionButton kind="destructive" label="Discard edits" icon="x" @press="onGuard('discard')" />
    </template>
  </BottomDrawer>
  <Dialog v-if="isDesktop" :open="confirming" @update:open="(next: boolean) => { confirming = next }">
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{{ dirtyTitle }}</DialogTitle>
        <DialogDescription>{{ dirtyBody }}</DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <ActionButton kind="secondary" label="Keep editing" icon="pencil" @press="onGuard('keep')" />
        <ActionButton kind="destructive" label="Discard edits" icon="x" @press="onGuard('discard')" />
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
