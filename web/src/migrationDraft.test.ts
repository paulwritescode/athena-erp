// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import MigrationDialog from './components/MigrationDialog.vue'
import type { SavePlan } from './api'
import type { RecordType } from './types'

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
})

Object.defineProperty(window, 'matchMedia', {
  configurable: true,
  value: () => ({
    matches: true,
    media: '',
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
    onchange: null,
  }),
})

function makeType(revision: number): RecordType {
  return {
    name: 'note',
    label: 'Note',
    description: '',
    kind: 'master',
    fields: [{ name: 'code', label: 'Code', type: 'Text', required: true }],
    revision,
  }
}

function makePlan(): SavePlan {
  return {
    requiresConfirmation: true,
    steps: [
      {
        kind: 'add_required_field',
        fieldKey: 'code',
        description: 'Add required field Code',
        destructive: false,
        affectedRecords: BigInt(3),
        needsBackfill: true,
      } as SavePlan['steps'][number],
    ],
  }
}

function backfillInput(): HTMLInputElement | null {
  return document.querySelector('#backfill-code')
}

async function mountDialog() {
  const plan = ref<SavePlan | null>(null)
  const recordType = ref<RecordType>(makeType(1))
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(MigrationDialog, { recordType: recordType.value, plan: plan.value }) })
  app.mount(el)
  cleanups.push(() => app.unmount())
  await nextTick()
  plan.value = makePlan()
  await nextTick()
  await nextTick()
  return { plan, recordType }
}

async function typeBackfill(value: string) {
  const input = backfillInput()
  if (!input) throw new Error('backfill input missing')
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}

describe('MigrationDialog draft', () => {
  it('keeps typed backfill across close and reopen of the same plan', async () => {
    const { plan } = await mountDialog()
    await typeBackfill('N/A')
    plan.value = null
    await nextTick()
    plan.value = makePlan()
    await nextTick()
    await nextTick()
    expect(backfillInput()?.value).toBe('N/A')
  })

  it('clears the draft after the save succeeds', async () => {
    const { plan, recordType } = await mountDialog()
    await typeBackfill('N/A')
    plan.value = null
    await nextTick()
    recordType.value = makeType(2)
    await nextTick()
    plan.value = makePlan()
    await nextTick()
    await nextTick()
    expect(backfillInput()?.value).toBe('')
  })

  it('clears the draft for a different plan', async () => {
    const { plan } = await mountDialog()
    await typeBackfill('N/A')
    plan.value = null
    await nextTick()
    const other = makePlan()
    other.steps = [
      {
        kind: 'add_required_field',
        fieldKey: 'code',
        description: 'Add required field Code for 5 records',
        destructive: false,
        affectedRecords: BigInt(5),
        needsBackfill: true,
      } as SavePlan['steps'][number],
    ]
    plan.value = other
    await nextTick()
    await nextTick()
    expect(backfillInput()?.value).toBe('')
  })
})
