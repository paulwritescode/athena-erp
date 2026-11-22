// Setup wizard step 2: logo (B11 slice 2). Uploads the light and dark
// Workspace logos through setIdentity; the B5 IdentitySection stays the
// full editor in Settings. Optional: skipping still finishes the wizard,
// and the readiness list shows the logo skipped.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { workspaceClient } from '@/api'
import { classify, describeFailure } from '@/apiErrors'
import { refreshIdentity } from '@/identity'
import ActionButton from '@/components/ActionButton.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import SetupFooter from './SetupFooter.vue'

const props = defineProps<{ canWrite: boolean }>()
const emit = defineEmits<{
  back: []
  done: [result: { hasLogo: boolean; skipped: boolean }]
  navigate: [page: 'settings']
}>()

const maxLogoBytes = 512 * 1024

const loaded = ref(false)
const loadError = ref('')
const saving = ref(false)
const editing = ref(false)
const currentLight = ref('')
const currentDark = ref('')
const lightFile = ref<File | null>(null)
const darkFile = ref<File | null>(null)
const lightPreview = ref('')
const darkPreview = ref('')
const lightProblem = ref('')
const darkProblem = ref('')
const saveProblem = ref('')
const lightPicker = ref<HTMLInputElement | null>(null)
const darkPicker = ref<HTMLInputElement | null>(null)

type Side = 'light' | 'dark'

function refs(side: Side) {
  return side === 'light'
    ? { file: lightFile, preview: lightPreview, problem: lightProblem }
    : { file: darkFile, preview: darkPreview, problem: darkProblem }
}

function shown(side: Side): string {
  const r = refs(side)
  if (r.file.value) return r.preview.value
  return side === 'light' ? currentLight.value : currentDark.value
}

const hasLogo = computed(() => currentLight.value !== '' || currentDark.value !== '')
const picked = computed(() => lightFile.value !== null || darkFile.value !== null)
// A saved logo shows the done panel; picking starts only when asked.
const showPickers = computed(() => !hasLogo.value || editing.value)

async function refresh() {
  loaded.value = false
  loadError.value = ''
  try {
    const res = await workspaceClient.getIdentity({})
    currentLight.value = res.identity?.logoLightUrl ?? ''
    currentDark.value = res.identity?.logoDarkUrl ?? ''
  } catch (err) {
    loadError.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not load the workspace logo.'
  } finally {
    loaded.value = true
  }
}

onMounted(() => {
  void refresh()
})

function pick(side: Side) {
  if (!props.canWrite) return
  const input = side === 'light' ? lightPicker.value : darkPicker.value
  if (!input) return
  refs(side).problem.value = ''
  saveProblem.value = ''
  input.click()
}

function chosen(side: Side, event: Event) {
  const input = event.target as HTMLInputElement | null
  const file = input?.files?.[0] ?? null
  if (input) input.value = ''
  if (!file) return
  const r = refs(side)
  const lower = file.name.toLowerCase()
  if (!lower.endsWith('.png') && !lower.endsWith('.svg')) {
    r.problem.value = `"${file.name}" is not a logo; use a .png or .svg file.`
    return
  }
  if (file.size > maxLogoBytes) {
    r.problem.value = `"${file.name}" is larger than 512 KB.`
    return
  }
  // A data: URL preview: blob: is blocked by the page img-src policy.
  const reader = new FileReader()
  reader.onload = () => {
    r.file.value = file
    r.preview.value = typeof reader.result === 'string' ? reader.result : ''
  }
  reader.onerror = () => {
    r.problem.value = `Could not read "${file.name}".`
  }
  reader.readAsDataURL(file)
}

function remove(side: Side) {
  const r = refs(side)
  r.file.value = null
  r.preview.value = ''
}

function setPicker(side: Side, el: unknown) {
  const input = el as HTMLInputElement | null
  if (side === 'light') lightPicker.value = input
  else darkPicker.value = input
}

async function fileBytes(file: File): Promise<Uint8Array> {
  return new Uint8Array(await file.arrayBuffer())
}

const primaryLabel = computed(() => {
  if (!props.canWrite || (hasLogo.value && !picked.value)) return 'Continue'
  return 'Save logo & continue'
})

const primaryDisabledReason = computed(() => {
  if (!props.canWrite || hasLogo.value) return null
  if (!picked.value) return 'Choose a file first, or skip for now.'
  return null
})

async function submit() {
  if (!props.canWrite || (hasLogo.value && !picked.value)) {
    emit('done', { hasLogo: hasLogo.value, skipped: !hasLogo.value })
    return
  }
  saveProblem.value = ''
  saving.value = true
  try {
    const res = await workspaceClient.setIdentity({
      displayName: '',
      ...(lightFile.value ? { logoLight: { content: await fileBytes(lightFile.value), filename: lightFile.value.name } } : {}),
      ...(darkFile.value ? { logoDark: { content: await fileBytes(darkFile.value), filename: darkFile.value.name } } : {}),
      clearLogoLight: false,
      clearLogoDark: false,
    })
    currentLight.value = res.identity?.logoLightUrl ?? currentLight.value
    currentDark.value = res.identity?.logoDarkUrl ?? currentDark.value
    await refreshIdentity()
    toast.success('Workspace logo saved.')
    emit('done', { hasLogo: true, skipped: false })
  } catch (err) {
    saveProblem.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not save the logo.'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <h2>Add the workspace logo</h2>
    <p class="text-muted-foreground mt-1 text-sm">It shows in the header, on sign-in and in the browser tab. Either theme, or both — or skip it for now.</p>
    <LoadingSkeleton v-if="!loaded" class="mt-6" />
    <div v-else-if="loadError" class="mt-6">
      <p class="text-sm" role="alert">{{ loadError }}</p>
      <SetupFooter primary-label="Continue" @back="emit('back')" @primary="emit('done', { hasLogo: false, skipped: false })">
        <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="refresh" />
      </SetupFooter>
    </div>
    <div v-else class="mt-6 space-y-4">
      <p v-if="!canWrite" class="muted-label">You need the workspace permission to change the logo.</p>
      <div v-for="side in (['light', 'dark'] as const)" :key="side" class="space-y-2">
        <p class="text-sm font-medium">{{ side === 'light' ? 'Light logo' : 'Dark logo' }}</p>
        <div class="flex flex-wrap items-center gap-3">
          <div
            class="flex h-18 w-36 items-center justify-center rounded-[var(--radius)] border p-3"
            :class="side === 'light' ? 'bg-white' : 'bg-zinc-900'"
          >
            <img
              v-if="shown(side)"
              :src="shown(side)"
              :alt="side === 'light' ? 'Light logo preview' : 'Dark logo preview'"
              class="max-h-12 w-auto max-w-full object-contain"
            />
            <span v-else class="text-muted-foreground text-sm">No logo</span>
          </div>
          <div v-if="canWrite && showPickers" class="flex flex-wrap items-center gap-2">
            <input
              :ref="(el) => setPicker(side, el)"
              class="hidden"
              type="file"
              accept=".png,.svg,image/png,image/svg+xml"
              :aria-label="side === 'light' ? 'Choose a light logo file' : 'Choose a dark logo file'"
              @change="chosen(side, $event)"
            />
            <ActionButton
              kind="secondary"
              density="row"
              :label="refs(side).file.value ? 'Replace' : shown(side) ? 'Replace' : 'Choose file'"
              icon="upload"
              @press="pick(side)"
            />
            <ActionButton
              v-if="refs(side).file.value"
              kind="quiet"
              density="row"
              label="Remove"
              icon="x"
              @press="remove(side)"
            />
          </div>
        </div>
        <p v-if="refs(side).problem.value" class="text-sm text-destructive" role="alert">{{ refs(side).problem.value }}</p>
        <p v-else-if="canWrite && showPickers" class="muted-label">PNG or SVG, at most 512 KB.</p>
      </div>
      <p v-if="saveProblem" class="text-sm text-destructive" role="alert">{{ saveProblem }}</p>
      <SetupFooter
        :primary-label="primaryLabel"
        :primary-icon="primaryLabel === 'Continue' ? 'chevron-right' : 'save'"
        :primary-busy="saving"
        :primary-disabled-reason="primaryDisabledReason"
        @back="emit('back')"
        @primary="submit"
      >
        <ActionButton
          v-if="canWrite && hasLogo && !editing"
          kind="quiet"
          label="Change"
          icon="pencil"
          @press="editing = true"
        />
        <ActionButton
          v-if="!hasLogo"
          kind="quiet"
          label="Skip for now"
          icon="chevron-right"
          @press="emit('done', { hasLogo: false, skipped: true })"
        />
        <ActionButton
          v-if="hasLogo"
          kind="quiet"
          label="Edit in Settings"
          icon="settings"
          @press="emit('navigate', 'settings')"
        />
      </SetupFooter>
    </div>
  </div>
</template>
