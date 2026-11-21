// Settings → Appearance (B4, §18): the Workspace accent as ink/red swatches
// or Custom with a color picker for the accent and the five chart colors,
// previewed in light and dark side by side before saving. The server owns
// contrast: on failure it suggests the nearest passing accent with a
// one-click "Use suggested". Read-only without settings:appearance.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { workspaceClient } from '../../api'
import { classify, describeFailure } from '../../apiErrors'
import { ACCENT_PRESETS, applyAppearance, hexToOklch, loadCachedAppearance, oklchToHex, previewCustom, saveCachedAppearance } from '../../appearance'
import type { Appearance } from '../../gen/athena/v1/workspace_pb'
import { AppearanceContrastFailureSchema } from '../../gen/athena/v1/workspace_pb'
import ActionButton from '@/components/ActionButton.vue'
import { Badge } from '@/components/ui/badge'

const props = defineProps<{ canEdit: boolean }>()

const loaded = ref(false)
const loadError = ref('')
const saving = ref(false)
const preset = ref('ink')
const accentHex = ref('')
const charts = ref<string[]>(['', '', '', '', ''])
const suggested = ref('')

const presets = ACCENT_PRESETS

// Empty chart slots fall through to the ink defaults; the picker shows the
// effective color so black never poses as a choice.
const INK_CHARTS = ['oklch(0.45 0.03 260)', 'oklch(0.62 0.09 55)', 'oklch(0.55 0.07 150)', 'oklch(0.50 0.08 240)', 'oklch(0.75 0.06 85)']
function chartDisplay(index: number): string {
  if (charts.value[index]) return charts.value[index]
  try {
    return oklchToHex(INK_CHARTS[index])
  } catch {
    return oklchToHex('oklch(0.45 0.03 260)')
  }
}

// The candidate the preview panes render, as oklch the server parses.
const candidate = computed(() => {
  if (preset.value !== 'custom') return null
  try {
    return { accent: hexToOklch(accentHex.value), charts: charts.value.map((hex) => (hex ? hexToOklch(hex) : '')) }
  } catch {
    return null
  }
})

const customPreview = computed(() => (candidate.value ? previewCustom(candidate.value.accent) : null))

function draftSignature(): string {
  return JSON.stringify({ preset: preset.value, accent: preset.value === 'custom' ? accentHex.value : '', charts: preset.value === 'custom' ? charts.value : [] })
}

const savedSignature = ref('')
const dirty = computed(() => loaded.value && draftSignature() !== savedSignature.value)

function fillFrom(appearance: Appearance) {
  preset.value = appearance.preset || 'ink'
  accentHex.value = appearance.preset === 'custom' && appearance.accent ? oklchToHex(appearance.accent) : oklchToHex('oklch(0.21 0.006 285.885)')
  charts.value = [0, 1, 2, 3, 4].map((i) => {
    const raw = appearance.chartColors[i] ?? ''
    if (!raw) return ''
    try {
      return oklchToHex(raw)
    } catch {
      return ''
    }
  })
  savedSignature.value = draftSignature()
}

async function refresh() {
  loaded.value = false
  loadError.value = ''
  try {
    const res = await workspaceClient.getAppearance({})
    if (res.appearance) fillFrom(res.appearance)
  } catch (err) {
    loadError.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not load the appearance.'
  } finally {
    loaded.value = true
  }
}

onMounted(refresh)

// Pane-scoped tokens for a custom candidate. Presets scope data-accent on
// the pane instead, so theme.css stays the single source for their values.
function paneVars(dark: boolean): Record<string, string> {
  const preview = customPreview.value
  if (!preview) return {}
  const primary = dark ? preview.dark : preview.light
  const vars: Record<string, string> = {
    '--primary': primary,
    '--primary-foreground': dark ? 'oklch(0.21 0.006 285.885)' : 'oklch(0.985 0 0)',
    '--sidebar-primary': primary,
    '--sidebar-primary-foreground': dark ? 'oklch(0.21 0.006 285.885)' : 'oklch(0.985 0 0)',
  }
  const list = charts.value
  vars['--chart-1'] = list[0] || primary
  for (let i = 1; i < 5; i++) if (list[i]) vars[`--chart-${i + 1}`] = list[i]
  return vars
}

async function persist(appearance: { preset: string; accent: string; chartColors: string[] }, done: string) {
  saving.value = true
  suggested.value = ''
  try {
    const res = await workspaceClient.setAppearance({ appearance })
    if (res.appearance) {
      const palette = { light: res.lightAccent, dark: res.darkAccent }
      applyAppearance({ preset: res.appearance.preset, accent: res.appearance.accent, chartColors: [...res.appearance.chartColors] }, palette)
      saveCachedAppearance(loadCachedAppearance()?.slug ?? '', res.appearance, palette)
      fillFrom(res.appearance)
    }
    toast.success(done)
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.InvalidArgument) {
      const detail = err.findDetails(AppearanceContrastFailureSchema)[0]
      if (detail?.suggestedAccent) {
        suggested.value = detail.suggestedAccent
        return
      }
    }
    toast.error(err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not save the appearance.')
  } finally {
    saving.value = false
  }
}

function save() {
  if (preset.value === 'custom') {
    if (!candidate.value) {
      toast.error('Pick an accent color first.')
      return
    }
    void persist({ preset: 'custom', accent: candidate.value.accent, chartColors: candidate.value.charts }, 'Appearance saved.')
  } else {
    void persist({ preset: preset.value, accent: '', chartColors: [] }, 'Appearance saved.')
  }
}

function useSuggested() {
  try {
    accentHex.value = oklchToHex(suggested.value)
  } catch {
    toast.error('That suggestion did not parse. Pick an accent instead.')
    return
  }
  preset.value = 'custom'
  save()
}

function reset() {
  void persist({ preset: 'ink', accent: '', chartColors: [] }, 'Back to the Ink preset.')
}
</script>

<template>
  <div v-if="!loaded" class="settings-loading" aria-label="Loading appearance">
    <div class="h-9 w-40 animate-pulse rounded-full bg-muted" />
    <div class="mt-4 grid gap-4 min-[640px]:grid-cols-2">
      <div class="h-48 animate-pulse rounded-[var(--radius)] bg-muted" />
      <div class="h-48 animate-pulse rounded-[var(--radius)] bg-muted" />
    </div>
  </div>
  <div v-else-if="loadError" class="empty-state">
    <p>{{ loadError }}</p>
    <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="refresh" />
  </div>
  <div v-else class="space-y-6">
    <div role="radiogroup" aria-label="Accent" class="flex flex-wrap gap-3">
      <button
        v-for="item in presets"
        :key="item.value"
        type="button"
        role="radio"
        :aria-checked="preset === item.value"
        :disabled="!canEdit"
        class="flex items-center gap-2 rounded-full border border-border py-1 pr-4 pl-1 disabled:cursor-not-allowed disabled:opacity-50 aria-checked:border-ring aria-checked:ring-2 aria-checked:ring-ring/30"
        @click="preset = item.value"
      >
        <span
          v-if="item.value !== 'custom'"
          :data-accent="item.value"
          class="size-8 rounded-full border border-black/10"
          :style="{ backgroundColor: 'var(--primary)' }"
          aria-hidden="true"
        />
        <span v-else class="size-8 rounded-full border border-black/10" :style="{ backgroundColor: accentHex }" aria-hidden="true" />
        <span class="text-sm font-medium">{{ item.label }}</span>
      </button>
    </div>
    <p v-if="!canEdit" class="muted-label">You need the appearance permission to change this.</p>

    <div v-if="preset === 'custom'" class="space-y-4">
      <div class="flex items-center gap-3">
        <input id="appearance-accent" v-model="accentHex" type="color" :disabled="!canEdit" aria-label="Accent color" class="h-10 w-14 shrink-0 cursor-pointer rounded-md border border-border bg-card p-1 disabled:cursor-not-allowed" />
        <div>
          <p class="text-sm font-medium">Accent</p>
          <p class="muted-label">{{ candidate?.accent ?? 'Pick a color.' }}</p>
        </div>
      </div>
      <div class="grid gap-3 min-[640px]:grid-cols-2">
        <div v-for="index in 5" :key="index" class="flex items-center gap-3">
          <input
            :id="`appearance-chart-${index}`"
            :model-value="chartDisplay(index - 1)"
            type="color"
            :disabled="!canEdit"
            :aria-label="`Chart ${index} color`"
            class="h-9 w-12 shrink-0 cursor-pointer rounded-md border border-border bg-card p-1 disabled:cursor-not-allowed"
            @input="charts[index - 1] = ($event.target as HTMLInputElement).value"
          />
          <p class="text-sm font-medium">Chart {{ index }}</p>
          <button
            v-if="charts[index - 1]"
            type="button"
            class="text-muted-foreground ml-auto text-sm underline-offset-4 hover:underline disabled:cursor-not-allowed"
            :disabled="!canEdit"
            @click="charts[index - 1] = ''"
          >
            Default
          </button>
        </div>
      </div>
      <p v-if="customPreview && !customPreview.passes" class="text-sm text-destructive">White text would be hard to read on this accent. Saving still tries the server.</p>
    </div>

    <div class="grid gap-4 min-[640px]:grid-cols-2">
      <div
        v-for="dark in [false, true]"
        :key="dark ? 'dark' : 'light'"
        :class="['rounded-[var(--radius)] border border-border bg-background p-4 text-foreground', dark ? 'dark' : undefined]"
        :data-accent="preset === 'custom' ? undefined : preset"
        :style="preset === 'custom' ? paneVars(dark) : undefined"
      >
        <p class="muted-label mb-3">{{ dark ? 'Dark' : 'Light' }}</p>
        <div class="space-y-3">
          <span class="inline-flex h-9 items-center rounded-full bg-primary px-4 text-sm font-medium text-primary-foreground">Save</span>
          <span class="block h-9 rounded-3xl border border-ring px-3 text-sm ring-3 ring-ring/30">Focus ring</span>
          <span class="block h-9 rounded-xl bg-sidebar-accent px-3 text-sm leading-9 text-sidebar-accent-foreground">Selected page</span>
          <span class="flex h-16 items-end gap-1" aria-hidden="true">
            <i class="w-full rounded-sm" style="height: 40%; background: var(--chart-1)" />
            <i class="w-full rounded-sm" style="height: 70%; background: var(--chart-2)" />
            <i class="w-full rounded-sm" style="height: 55%; background: var(--chart-3)" />
            <i class="w-full rounded-sm" style="height: 85%; background: var(--chart-4)" />
            <i class="w-full rounded-sm" style="height: 30%; background: var(--chart-5)" />
          </span>
        </div>
      </div>
    </div>

    <div v-if="suggested" class="flex flex-wrap items-center gap-3 rounded-[var(--radius)] border border-border p-3">
      <span class="size-8 shrink-0 rounded-full border border-black/10" :style="{ backgroundColor: suggested }" aria-hidden="true" />
      <p class="min-w-0 flex-1 text-sm">Text is hard to read on that accent. Try {{ suggested }}.</p>
      <ActionButton kind="secondary" label="Use suggested" icon="check" :busy="saving" @press="useSuggested" />
    </div>

    <div v-if="canEdit" class="flex flex-wrap gap-2">
      <ActionButton kind="primary" label="Save appearance" icon="save" :busy="saving" :disabled-reason="dirty ? null : 'No changes to save.'" @press="save" />
      <ActionButton kind="quiet" label="Reset to default" icon="rotate-ccw" :busy="saving" @press="reset" />
    </div>
    <div v-else class="flex flex-wrap gap-2">
      <Badge variant="secondary">Read-only</Badge>
    </div>
  </div>
</template>
