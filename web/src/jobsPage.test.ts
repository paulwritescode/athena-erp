// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import JobsPage from './pages/JobsPage.vue'
import { eventClient, jobClient } from './api'

vi.mock('./api', () => ({
  eventClient: { listEvents: vi.fn(), getEvent: vi.fn(), listOutbox: vi.fn(), requeueOutbox: vi.fn(), listSubscriptions: vi.fn() },
  jobClient: { listJobs: vi.fn(), getJob: vi.fn(), retryJob: vi.fn(), listOccurrences: vi.fn() },
}))

const deadJob = {
  id: '11111111-1111-4111-8111-111111111111', kind: 'probe-a', status: 'dead',
  attempts: 3, maxAttempts: 3, nextRunAt: '', leaseOwner: '', leaseExpiresAt: '',
  fencing: BigInt(4), occurrenceKey: '', ownerUserId: '', ownerRole: 'Builder',
  lastError: 'boom', payload: '', result: '', createdAt: '2026-11-23T10:00:00Z', updatedAt: '2026-11-23T10:05:00Z',
}
const queuedJob = {
  id: '22222222-2222-4222-8222-222222222222', kind: 'probe-b', status: 'queued',
  attempts: 0, maxAttempts: 5, nextRunAt: '', leaseOwner: '', leaseExpiresAt: '',
  fencing: BigInt(0), occurrenceKey: '', ownerUserId: '', ownerRole: '',
  lastError: '', payload: '', result: '', createdAt: '2026-11-23T11:00:00Z', updatedAt: '2026-11-23T11:00:00Z',
}
const fullDeadJob = { ...deadJob, payload: '{"marker":"job-one"}', result: '' }

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  // resetAllMocks, not restoreAllMocks: a test that throws can leave a
  // queued once-implementation behind, which the next test would consume.
  vi.resetAllMocks()
})

async function settle() {
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 20))
  await nextTick()
}

async function mountPage() {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(JobsPage) })
  app.mount(el)
  cleanups.push(() => app.unmount())
  await settle()
}

function tabTrigger(label: string): HTMLButtonElement {
  const found = [...document.querySelectorAll('button[role="tab"]')].find((b) => b.textContent?.trim() === label)
  if (!found) throw new Error(`missing tab ${label}`)
  return found as HTMLButtonElement
}

// Reka tabs activate on mousedown or focus, not on click.
function openTab(label: string) {
  const trigger = tabTrigger(label)
  trigger.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0 }))
  trigger.focus()
}

describe('JobsPage', () => {
  it('loads jobs newest first and pages forward', async () => {
    vi.mocked(jobClient.listJobs)
      .mockResolvedValueOnce({ jobs: [queuedJob], nextPageToken: 'tok' } as never)
      .mockResolvedValueOnce({ jobs: [deadJob], nextPageToken: '' } as never)
    await mountPage()
    expect(document.body.textContent).toContain('probe-b')
    expect(vi.mocked(jobClient.listJobs).mock.calls[0][0]).toMatchObject({ status: '', pageSize: 25, pageToken: '' })
    const next = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes('Next'))
    if (!next) throw new Error('missing Next button')
    ;(next as HTMLButtonElement).click()
    await settle()
    expect(vi.mocked(jobClient.listJobs).mock.calls[1][0]).toMatchObject({ pageToken: 'tok' })
    expect(document.body.textContent).toContain('probe-a')
  })

  it('shows the empty state when no jobs are queued', async () => {
    vi.mocked(jobClient.listJobs).mockResolvedValue({ jobs: [], nextPageToken: '' } as never)
    await mountPage()
    expect(document.body.textContent).toContain('No jobs yet')
  })

  it('shows a retryable error when loading fails', async () => {
    vi.mocked(jobClient.listJobs)
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValueOnce({ jobs: [queuedJob], nextPageToken: '' } as never)
    await mountPage()
    expect(document.body.textContent).toContain('Could not load the jobs')
    const retry = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes('Try again'))
    if (!retry) throw new Error('missing Try again button')
    ;(retry as HTMLButtonElement).click()
    await settle()
    expect(document.body.textContent).toContain('probe-b')
  })

  it('opens a job, fills its payload and retries it', async () => {
    vi.mocked(jobClient.listJobs).mockResolvedValue({ jobs: [deadJob, queuedJob], nextPageToken: '' } as never)
    vi.mocked(jobClient.getJob).mockResolvedValue({ job: fullDeadJob } as never)
    vi.mocked(jobClient.retryJob).mockResolvedValue({ jobId: '33333333-3333-4333-8333-333333333333' } as never)
    await mountPage()
    const row = [...document.querySelectorAll('tbody tr')].find((tr) => tr.textContent?.includes('probe-a'))
    if (!row) throw new Error('missing probe-a row')
    ;(row as HTMLElement).click()
    await settle()
    expect(vi.mocked(jobClient.getJob)).toHaveBeenCalledWith({ id: deadJob.id })
    expect(document.body.textContent).toContain('job-one')
    const retry = [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Retry')
    if (!retry) throw new Error('missing sheet Retry button')
    ;(retry as HTMLButtonElement).click()
    await settle()
    expect(vi.mocked(jobClient.retryJob)).toHaveBeenCalledWith({ id: deadJob.id })
    expect(vi.mocked(jobClient.listJobs).mock.calls.length).toBeGreaterThan(1)
  })

  it('loads each tab lazily on first open', async () => {
    vi.mocked(jobClient.listJobs).mockResolvedValue({ jobs: [], nextPageToken: '' } as never)
    vi.mocked(eventClient.listEvents).mockResolvedValue({
      events: [{
        id: 'e1', sequence: BigInt(7), eventType: 'posting.posted', schemaVersion: 1,
        workspace: '', producer: 'posting', subjectType: 'bill', subjectId: 'b1',
        subjectRevision: BigInt(2), actor: 'bob', operationId: '', correlationId: 'c1',
        causationId: '', payload: '{}', createdAt: '2026-11-23T10:00:00Z',
      }], nextPageToken: '',
    } as never)
    vi.mocked(eventClient.listOutbox).mockResolvedValue({ rows: [], nextPageToken: '' } as never)
    vi.mocked(jobClient.listOccurrences).mockResolvedValue({ occurrences: [] } as never)
    vi.mocked(eventClient.listSubscriptions).mockResolvedValue({ subscriptions: [] } as never)
    await mountPage()
    expect(vi.mocked(eventClient.listEvents)).not.toHaveBeenCalled()
    openTab('Events')
    await settle()
    expect(vi.mocked(eventClient.listEvents)).toHaveBeenCalledTimes(1)
    expect(document.body.textContent).toContain('posting.posted')
    openTab('Outbox')
    await settle()
    expect(vi.mocked(eventClient.listOutbox)).toHaveBeenCalledTimes(1)
    openTab('Schedules')
    await settle()
    expect(vi.mocked(jobClient.listOccurrences)).toHaveBeenCalledTimes(1)
    openTab('Subscriptions')
    await settle()
    expect(vi.mocked(eventClient.listSubscriptions)).toHaveBeenCalledTimes(1)
  })
})
