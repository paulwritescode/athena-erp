// Settings → Workspace identity (B5, §10): the display name and the light
// and dark logos. Logos preview at 48 px with a quarter-height clear space,
// each on its own theme. Read-only without settings:workspace.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { workspaceClient } from '../../api'
import { classify, describeFailure } from '../../apiErrors'
import { refreshIdentity } from '../../identity'
import type { Identity } from '../../gen/athena/v1/workspace_pb'
import ActionButton from '@/components/ActionButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import TextField from '@/components/fields/TextField.vue'
import { IdCard } from '@/icons'

const props = defineProps<{ canEdit: boolean }>()

const maxLogoBytes = 512 * 1024

const loaded = ref(false)
const loadError = ref('')
const saving = ref(false)
const name = ref('')
const savedName = ref('')
const currentLight = ref('')
const currentDark = ref('')
const lightFile = ref<File | null>(null)
const darkFile = ref<File | null>(null)
const lightPreview = ref('')
const darkPreview = ref('')
const clearLight = ref(false)
const clearDark = ref(false)
const nameProblem = ref('')
const lightProblem = ref('')
const darkProblem = ref('')
const lightPicker = ref<HTMLInputElement | null>(null)
const darkPicker = ref<HTMLInputElement | null>(null)

type Side = 'light' | 'dark'

function refs(side: Side) {
  return side === 'light'
    ? { file: lightFile, preview: lightPreview, clear: clearLight, problem: lightProblem }
    : { file: darkFile, preview: darkPreview, clear: clearDark, problem: darkProblem }
}

function shown(side: Side): string {
  const r = refs(side)
  if (r.file.value) return r.preview.value
  if (r.clear.value) return ''
  return side === 'light' ? currentLight.value : currentDark.value
}

const dirty = computed(
  () =>
    name.value.trim() !== savedName.value ||
    lightFile.value !== null ||
    darkFile.value !== null ||
    clearLight.value ||
    clearDark.value,
)

function fillFrom(identity: Identity | undefined) {
  savedName.value = identity?.displayName ?? ''
  name.value = savedName.value
  currentLight.value = identity?.logoLightUrl ?? ''
  currentDark.value = identity?.logoDarkUrl ?? ''
  for (const side of ['light', 'dark'] as const) {
    const r = refs(side)
    if (r.preview.value.startsWith('blob:')) URL.revokeObjectURL(r.preview.value)
    r.file.value = null
    r.preview.value = ''
    r.clear.value = false
    r.problem.value = ''
  }
  nameProblem.value = ''
}

async function refresh() {
  loaded.value = false
  loadError.value = ''
  try {
    const res = await workspaceClient.getIdentity({})
    fillFrom(res.identity)
  } catch (err) {
    loadError.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not load the Workspace identity.'
  } finally {
    loaded.value = true
  }
}

onMounted(() => {
  void refresh()
})

function setPicker(side: Side, el: unknown) {
  const input = el as HTMLInputElement | null
  if (side === 'light') lightPicker.value = input
  else darkPicker.value = input
}

function pick(side: Side) {
  if (!props.canEdit) return
  const input = side === 'light' ? lightPicker.value : darkPicker.value
  if (!input) return
  refs(side).problem.value = ''
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
  if (r.preview.value.startsWith('blob:')) URL.revokeObjectURL(r.preview.value)
  r.file.value = file
  r.preview.value = URL.createObjectURL(file)
  r.clear.value = false
}

function remove(side: Side) {
  const r = refs(side)
  if (r.file.value) {
    if (r.preview.value.startsWith('blob:')) URL.revokeObjectURL(r.preview.value)
    r.file.value = null
    r.preview.value = ''
    return
  }
  r.clear.value = true
}

function undoRemove(side: Side) {
  refs(side).clear.value = false
}

async function fileBytes(file: File): Promise<Uint8Array> {
  return new Uint8Array(await file.arrayBuffer())
}

async function save() {
  const trimmed = name.value.trim()
  if ([...trimmed].length > 80) {
    nameProblem.value = 'The display name holds 1 to 80 characters.'
    return
  }
  saving.value = true
  try {
    const res = await workspaceClient.setIdentity({
      displayName: trimmed !== savedName.value ? trimmed : '',
      ...(lightFile.value ? { logoLight: { content: await fileBytes(lightFile.value), filename: lightFile.value.name } } : {}),
      ...(darkFile.value ? { logoDark: { content: await fileBytes(darkFile.value), filename: darkFile.value.name } } : {}),
      clearLogoLight: clearLight.value,
      clearLogoDark: clearDark.value,
    })
    fillFrom(res.identity)
    await refreshIdentity()
    toast.success('Workspace identity saved.')
  } catch (err) {
    toast.error(err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not save the Workspace identity.')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div v-if="!loaded" class="settings-loading" aria-label="Loading Workspace identity">
    <div class="h-9 w-full animate-pulse rounded-3xl bg-muted" />
    <div class="mt-3 h-9 w-full animate-pulse rounded-3xl bg-muted" />
  </div>
  <div v-else-if="loadError" class="empty-state">
    <p>{{ loadError }}</p>
    <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="refresh" />
  </div>
  <div v-else class="space-y-4">
    <p v-if="!canEdit" class="muted-label">You need the workspace permission to change this.</p>
    <TextField
      id="identity-name"
      label="Display name"
      help="The name people see in the header, on sign-in and in the browser tab."
      :error="nameProblem || undefined"
      :model-value="name"
      :disabled="!canEdit"
      @update:model-value="name = $event; nameProblem = ''"
    />
    <div v-for="side in (['light', 'dark'] as const)" :key="side" class="space-y-2">
      <p class="text-sm font-medium">{{ side === 'light' ? 'Light logo' : 'Dark logo' }}</p>
      <div class="flex flex-wrap items-center gap-3">
        <div
          class="flex h-18 w-36 items-center justify-center rounded-[var(--radius)] border p-3"
          :class="side === 'light' ? 'bg-white' : 'bg-zinc-900'"
        >
          <img v-if="shown(side)" :src="shown(side)" :alt="side === 'light' ? 'Light logo preview' : 'Dark logo preview'" class="max-h-12 w-auto max-w-full object-contain" />
          <EmptyState v-else :icon="IdCard" title="No logo" body="" class="preview-empty" />
        </div>
        <div class="flex flex-wrap items-center gap-2">
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
            :label="shown(side) ? 'Replace' : 'Choose file'"
            icon="upload"
            :disabled-reason="canEdit ? null : 'You need the workspace permission to change this.'"
            @press="pick(side)"
          />
          <ActionButton
            v-if="shown(side) || refs(side).clear.value"
            kind="quiet"
            density="row"
            :label="refs(side).clear.value ? 'Keep logo' : 'Remove'"
            :icon="refs(side).clear.value ? 'undo-2' : 'x'"
            :disabled-reason="canEdit ? null : 'You need the workspace permission to change this.'"
            @press="refs(side).clear.value ? undoRemove(side) : remove(side)"
          />
        </div>
      </div>
      <p v-if="refs(side).problem.value" class="text-sm text-destructive" role="alert">{{ refs(side).problem.value }}</p>
      <p v-else class="muted-label">PNG or SVG, at most 512 KB. Without one the Athena mark shows.</p>
    </div>
    <div v-if="canEdit" class="flex flex-wrap gap-2 pt-2">
      <ActionButton
        kind="primary"
        label="Save identity"
        icon="save"
        :busy="saving"
        :disabled-reason="!dirty ? 'No changes to save.' : null"
        @press="save"
      />
    </div>
  </div>
</template>

<style scoped>
.preview-empty {
  padding: 0;
}
.preview-empty :deep(h2) {
  font-size: 13px;
  margin-top: 8px;
}
.preview-empty :deep(p) {
  display: none;
}
</style>
