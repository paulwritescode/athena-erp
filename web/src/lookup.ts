import { recordClient } from './api'
import type { LookupItem } from './gen/athena/v1/records_pb'

// Pickers show a Record by its label and store its ID. Labels of Records a
// page has already seen are kept here, so a form or table never asks again for
// a label it holds. A write to a Record Type calls forgetLabels for it.
const labels = new Map<string, Map<string, LookupItem>>()

function bucket(recordTypeKey: string): Map<string, LookupItem> {
  let items = labels.get(recordTypeKey)
  if (!items) labels.set(recordTypeKey, (items = new Map()))
  return items
}

/** Drops the remembered labels of one Record Type after a write to it. */
export function forgetLabels(recordTypeKey: string): void {
  labels.delete(recordTypeKey)
}

/** One page of Records that can be chosen now, filtered by label text. */
export async function searchRecords(recordTypeKey: string, query: string, pageToken = ''): Promise<{ items: LookupItem[]; nextPageToken: string }> {
  const res = await recordClient.lookupRecords({ recordTypeKey, query, pageSize: 20, pageToken })
  const known = bucket(recordTypeKey)
  for (const item of res.items) known.set(item.id, item)
  return { items: res.items, nextPageToken: res.nextPageToken }
}

/**
 * Labels for the given IDs, whatever state the Records are in (a cancelled or
 * deleted Record keeps its label and carries a state). IDs the server does
 * not know are left out.
 */
export async function resolveRecords(recordTypeKey: string, ids: string[]): Promise<Map<string, LookupItem>> {
  const known = bucket(recordTypeKey)
  const missing = [...new Set(ids)].filter((id) => id && !known.has(id))
  for (let start = 0; start < missing.length; start += 50) {
    const res = await recordClient.lookupRecords({ recordTypeKey, ids: missing.slice(start, start + 50) })
    for (const item of res.items) known.set(item.id, item)
  }
  return new Map(ids.flatMap((id) => (known.has(id) ? [[id, known.get(id)!] as const] : [])))
}

/** The label with its state, such as "Acme (cancelled)". */
export function labelWithState(item: LookupItem): string {
  return item.state ? `${item.label} (${item.state})` : item.label
}
