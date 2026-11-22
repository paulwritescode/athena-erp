// Settings → Appearance (B4b, §18): a 280 px column of choices (Accent,
// Charts, Your theme) opening a popover on desktop and a bottom drawer on
// touch, beside a realistic preview card in light and dark. The server owns
// validation and contrast: on failure it suggests the nearest passing
// accent with a one-click "Use suggested". The Workspace choices are
// read-only without settings:appearance; the theme is per user and always
// editable.
<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { Check, Monitor, Moon, Sun } from '@/icons'
import { workspaceClient } from '../../api'
import { classify, describeFailure } from '../../apiErrors'
import {
  ACCENT_PRESETS,
  CHART_PALETTES,
  MATCH_RAMP_VARS,
  MATCH_RAMP_VARS_DARK,
  PRESET_ACCENTS,
  applyAppearance,
  hexToOklch,
  loadCachedAppearance,
  oklchToHex,
  previewCustom,
  rampFallbackHex,
  saveCachedAppearance,
} from '../../appearance'
import type { ThemeState } from '../../composables/useTheme'
import type { Appearance } from '../../gen/athena/v1/workspace_pb'
import { AppearanceContrastFailureSchema } from '../../gen/athena/v1/workspace_pb'
import ActionButton from '@/components/ActionButton.vue'
import { Badge } from '@/components/ui/badge'
import AppearanceChoiceRow from './AppearanceChoiceRow.vue'
import AppearancePreview from './AppearancePreview.vue'

const props = defineProps<{ canEdit: boolean }>()

const theme = inject<ThemeState | undefined>('athena-theme', undefined)

const loaded = ref(false)
const loadError = ref('')
const saving = ref(false)
const preset = ref('ink')
const accentHex = ref('')
const chartPalette = ref('match')
const charts = ref<string[]>(['', '', '', '', ''])
const suggested = ref('')

// Empty chart slots fall through to the accent ramp; the picker shows that
// ramp for the draft accent, so black never poses as a choice and the ink
// ramp never stands in for another accent.
const draftAccent = computed(() => {
  if (preset.value === 'custom') return candidate.value?.accent || PRESET_ACCENTS.ink
  return PRESET_ACCENTS[preset.value] ?? PRESET_ACCENTS.ink
})
function chartDisplay(index: number): string {
  return charts.value[index] || rampFallbackHex(draftAccent.value, index)
}

const accentLabel = computed(() => ACCENT_PRESETS.find((p) => p.value === preset.value)?.label ?? 'Ink')
const chartLabel = computed(() => CHART_PALETTES.find((p) => p.value === chartPalette.value)?.label ?? 'Match accent')

const themeModes = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'System', icon: Monitor },
] as const

// The custom candidate as oklch the server parses.
const candidate = computed(() => {
  try {
    return {
      accent: preset.value === 'custom' ? hexToOklch(accentHex.value) : '',
      charts: chartPalette.value === 'custom' ? charts.value.map((hex) => (hex ? hexToOklch(hex) : '')) : [],
    }
  } catch {
    return null
  }
})

const customPreview = computed(() => (candidate.value?.accent ? previewCustom(candidate.value.accent) : null))

function draftSignature(): string {
  return JSON.stringify({
    preset: preset.value,
    accent: preset.value === 'custom' ? accentHex.value : '',
    chartPalette: chartPalette.value,
    charts: chartPalette.value === 'custom' ? charts.value : [],
  })
}

const savedSignature = ref('')
const dirty = computed(() => loaded.value && draftSignature() !== savedSignature.value)

function fillFrom(appearance: Appearance) {
  preset.value = appearance.preset || 'ink'
  chartPalette.value = appearance.chartPalette || (appearance.chartColors.length > 0 ? 'custom' : 'match')
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

// Preview scope: named choices scope data attributes on the pane so
// theme.css stays the single source for their values; custom values arrive
// as inline custom properties, per theme for the accent.
function paneAccent(): string | undefined {
  return preset.value === 'custom' ? undefined : preset.value
}

function pickAccent(value: string, close: () => void) {
  preset.value = value
  if (value !== 'custom') close()
}

function pickCharts(value: string, close: () => void) {
  chartPalette.value = value
  if (value !== 'custom') close()
}

function paneCharts(): string | undefined {
  return chartPalette.value === 'custom' || chartPalette.value === 'match' ? undefined : chartPalette.value
}

function paneVars(dark: boolean): Record<string, string> | undefined {
  const vars: Record<string, string> = {}
  const preview = customPreview.value
  if (preset.value === 'custom' && preview) {
    const primary = dark ? preview.dark : preview.light
    vars['--primary'] = primary
    vars['--primary-foreground'] = dark ? 'oklch(0.21 0.006 285.885)' : 'oklch(0.985 0 0)'
    vars['--sidebar-primary'] = primary
    vars['--sidebar-primary-foreground'] = dark ? 'oklch(0.21 0.006 285.885)' : 'oklch(0.985 0 0)'
  }
  if (chartPalette.value === 'custom' && candidate.value) {
    if (preset.value === 'custom') Object.assign(vars, dark ? MATCH_RAMP_VARS_DARK : MATCH_RAMP_VARS)
    candidate.value.charts.forEach((value, i) => {
      if (value) vars[`--chart-${i + 1}`] = value
    })
  } else if (preset.value === 'custom' && paneCharts() === undefined) {
    // A custom accent scopes no data-accent, so the ramp would resolve
    // against the saved accent: declare it on the pane instead. Named
    // palettes arrive as data-charts and need nothing inline.
    Object.assign(vars, dark ? MATCH_RAMP_VARS_DARK : MATCH_RAMP_VARS)
  }
  return Object.keys(vars).length > 0 ? vars : undefined
}

// The single phone pane follows the theme switch.
const initialDark = document.documentElement.classList.contains('dark')
const pageDark = computed(() => theme?.isDark.value ?? initialDark)
const phoneDark = pageDark
// The theme class scoped dots carry: a data-accent scope without it reads
// the light primaries even on a dark page (the Ink dot goes invisible).
const dotTheme = computed(() => (pageDark.value ? 'dark' : undefined))

const previewCaption = computed(() => `${accentLabel.value} with ${chartLabel.value.toLowerCase()} charts`)

// The Charts row indicator: five dots in the draft's effective ramp.
const rampDots = computed(() => {
  const out: { accent?: string; charts?: string; style?: Record<string, string> } = {}
  if (preset.value !== 'custom') out.accent = preset.value
  const named = paneCharts()
  if (named) out.charts = named
  const vars = paneVars(pageDark.value)
  if (vars) out.style = vars
  return out
})

async function persist(appearance: { preset: string; accent: string; chartPalette: string; chartColors: string[] }, done: string) {
  saving.value = true
  suggested.value = ''
  try {
    const res = await workspaceClient.setAppearance({ appearance })
    if (res.appearance) {
      const palette = { light: res.lightAccent, dark: res.darkAccent }
      applyAppearance(
        { preset: res.appearance.preset, accent: res.appearance.accent, chartPalette: res.appearance.chartPalette, chartColors: [...res.appearance.chartColors] },
        palette,
      )
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
  if (!candidate.value || (preset.value === 'custom' && !candidate.value.accent)) {
    toast.error('Pick an accent color first.')
    return
  }
  void persist(
    {
      preset: preset.value,
      accent: preset.value === 'custom' ? candidate.value.accent : '',
      chartPalette: chartPalette.value,
      chartColors: chartPalette.value === 'custom' ? candidate.value.charts : [],
    },
    'Appearance saved.',
  )
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
  void persist({ preset: 'ink', accent: '', chartPalette: 'match', chartColors: [] }, 'Back to the Ink preset.')
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
  <div v-else class="grid gap-6 min-[1024px]:grid-cols-[280px_minmax(0,1fr)]">
    <div class="space-y-3">
      <AppearanceChoiceRow label="Accent" :value="accentLabel" :disabled="!canEdit" drawer-title="Accent" popover-class="w-80 p-3">
        <template #indicator>
          <span
            v-if="preset !== 'custom'"
            :data-accent="preset"
            class="size-6 shrink-0 rounded-full border border-black/10"
            :class="dotTheme"
            :style="{ backgroundColor: 'var(--primary)' }"
            aria-hidden="true"
          />
          <span v-else class="size-6 shrink-0 rounded-full border border-black/10" :style="{ backgroundColor: accentHex }" aria-hidden="true" />
        </template>
        <template #options="{ close }">
          <div role="radiogroup" aria-label="Accent" class="grid grid-cols-5 gap-1">
            <button
              v-for="item in ACCENT_PRESETS"
              :key="item.value"
              type="button"
              role="radio"
              :aria-checked="preset === item.value"
              class="flex flex-col items-center gap-1 rounded-[var(--radius)] p-2 hover:bg-muted aria-checked:bg-muted aria-checked:ring-2 aria-checked:ring-ring/30"
              @click="pickAccent(item.value, close)"
            >
              <span
                v-if="item.value !== 'custom'"
                :data-accent="item.value"
                class="size-8 rounded-full border border-black/10"
                :class="dotTheme"
                :style="{ backgroundColor: 'var(--primary)' }"
                aria-hidden="true"
              />
              <span v-else class="size-8 rounded-full border border-black/10" :style="{ backgroundColor: accentHex }" aria-hidden="true" />
              <span class="text-xs">{{ item.label }}</span>
            </button>
          </div>
          <div v-if="preset === 'custom'" class="mt-3 flex items-center gap-3 border-t border-border pt-3">
            <input id="appearance-accent" v-model="accentHex" type="color" aria-label="Custom accent color" class="h-10 w-14 shrink-0 cursor-pointer rounded-md border border-border bg-card p-1" />
            <div class="min-w-0">
              <p class="text-sm font-medium">Custom</p>
              <p class="muted-label truncate">{{ candidate?.accent ?? 'Pick a color.' }}</p>
            </div>
          </div>
          <p v-if="preset === 'custom' && customPreview && !customPreview.passes" class="mt-2 text-sm text-destructive">
            White text would be hard to read on this accent. Saving still tries the server.
          </p>
        </template>
      </AppearanceChoiceRow>

      <AppearanceChoiceRow label="Charts" :value="chartLabel" :disabled="!canEdit" drawer-title="Charts" popover-class="w-80 p-2">
        <template #indicator>
          <span class="flex shrink-0 items-center gap-0.5" :class="dotTheme" :data-accent="rampDots.accent" :data-charts="rampDots.charts" :style="rampDots.style" aria-hidden="true">
            <i v-for="i in 5" :key="i" class="size-2 rounded-full" :style="{ background: `var(--chart-${i})` }" />
          </span>
        </template>
        <template #options="{ close }">
          <div role="radiogroup" aria-label="Charts" class="space-y-1">
            <button
              v-for="item in CHART_PALETTES"
              :key="item.value"
              type="button"
              role="radio"
              :aria-checked="chartPalette === item.value"
              class="flex w-full items-center gap-3 rounded-[var(--radius)] p-2 text-left hover:bg-muted aria-checked:bg-muted aria-checked:ring-2 aria-checked:ring-ring/30"
              @click="pickCharts(item.value, close)"
            >
              <span
                class="flex shrink-0 items-center gap-0.5"
                :class="dotTheme"
                :data-accent="item.value === 'match' ? paneAccent() : undefined"
                :data-charts="item.value === 'match' || item.value === 'custom' ? undefined : item.value"
                :style="item.value === 'match' ? paneVars(pageDark) : undefined"
                aria-hidden="true"
              >
                <i v-for="i in 5" :key="i" class="size-2.5 rounded-full" :style="{ background: `var(--chart-${i})` }" />
              </span>
              <span class="min-w-0 flex-1">
                <span class="block text-sm font-medium">{{ item.label }}</span>
                <span class="muted-label block">{{ item.blurb }}</span>
              </span>
              <Check v-if="chartPalette === item.value" class="size-4 shrink-0 text-foreground" aria-hidden="true" />
            </button>
          </div>
          <div v-if="chartPalette === 'custom'" class="mt-2 space-y-2 border-t border-border pt-3">
            <div v-for="index in 5" :key="index" class="flex items-center gap-3">
              <input
                :id="`appearance-chart-${index}`"
                :model-value="chartDisplay(index - 1)"
                type="color"
                :aria-label="`Chart ${index} color`"
                class="h-9 w-12 shrink-0 cursor-pointer rounded-md border border-border bg-card p-1"
                @input="charts[index - 1] = ($event.target as HTMLInputElement).value"
              />
              <p class="text-sm font-medium">Chart {{ index }}</p>
              <button
                v-if="charts[index - 1]"
                type="button"
                class="text-muted-foreground ml-auto text-sm underline-offset-4 hover:underline"
                @click="charts[index - 1] = ''"
              >
                Default
              </button>
            </div>
          </div>
        </template>
      </AppearanceChoiceRow>

      <AppearanceChoiceRow v-if="theme" label="Your theme" :value="theme.modeLabel.value" drawer-title="Your theme" popover-class="w-64 p-2">
        <template #indicator>
          <component :is="themeModes.find((m) => m.value === theme!.mode.value)?.icon" class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        </template>
        <template #options="{ close }">
          <p class="muted-label px-2 pt-1 pb-2">Only you see this — it never changes the Workspace.</p>
          <div role="radiogroup" aria-label="Your theme" class="space-y-1">
            <button
              v-for="item in themeModes"
              :key="item.value"
              type="button"
              role="radio"
              :aria-checked="theme!.mode.value === item.value"
              class="flex w-full items-center gap-3 rounded-[var(--radius)] p-2 text-left hover:bg-muted aria-checked:bg-muted aria-checked:ring-2 aria-checked:ring-ring/30"
              @click="theme!.setMode(item.value); close()"
            >
              <component :is="item.icon" class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              <span class="flex-1 text-sm font-medium">{{ item.label }}</span>
              <Check v-if="theme!.mode.value === item.value" class="size-4 shrink-0 text-foreground" aria-hidden="true" />
            </button>
          </div>
        </template>
      </AppearanceChoiceRow>
      <p v-if="!canEdit" class="muted-label">You need the appearance permission to change the Workspace choices.</p>

      <div v-if="suggested && canEdit" class="flex flex-wrap items-center gap-3 rounded-[var(--radius)] border border-border p-3">
        <span class="size-8 shrink-0 rounded-full border border-black/10" :style="{ backgroundColor: suggested }" aria-hidden="true" />
        <p class="min-w-0 flex-1 text-sm">Text is hard to read on that accent. Try {{ suggested }}.</p>
        <ActionButton kind="secondary" label="Use suggested" icon="check" :busy="saving" @press="useSuggested" />
      </div>

      <div v-if="canEdit" class="flex flex-wrap gap-2">
        <ActionButton kind="primary" label="Save" icon="save" :busy="saving" :disabled-reason="dirty ? null : 'No changes to save.'" @press="save" />
        <ActionButton kind="quiet" label="Reset to default" icon="rotate-ccw" :busy="saving" @press="reset" />
      </div>
      <div v-else class="flex flex-wrap gap-2">
        <Badge variant="secondary">Read-only</Badge>
      </div>
    </div>

    <div>
      <div class="min-[640px]:hidden">
        <p class="muted-label mb-2">{{ phoneDark ? 'Dark' : 'Light' }}</p>
        <AppearancePreview
          :force="phoneDark ? 'dark' : 'light'"
          :accent="paneAccent()"
          :charts="paneCharts()"
          :custom-vars="paneVars(phoneDark)"
          :caption="previewCaption"
        />
      </div>
      <div class="hidden gap-4 min-[640px]:grid min-[640px]:grid-cols-2">
        <div>
          <p class="muted-label mb-2">Light</p>
          <AppearancePreview force="light" :accent="paneAccent()" :charts="paneCharts()" :custom-vars="paneVars(false)" :caption="previewCaption" />
        </div>
        <div>
          <p class="muted-label mb-2">Dark</p>
          <AppearancePreview force="dark" :accent="paneAccent()" :charts="paneCharts()" :custom-vars="paneVars(true)" :caption="previewCaption" />
        </div>
      </div>
    </div>
  </div>
</template>
