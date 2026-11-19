import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { SystemService } from './gen/athena/v1/system_pb'
import { EntryService } from './gen/athena/v1/entries_pb'
import { RecordService, RecordSchema, type MigrationStep, type Record as RecordMessage } from './gen/athena/v1/records_pb'
import type { FieldValue } from './gen/athena/v1/entries_pb'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import type { RecordDefinition } from './types'

// All ConnectRPC calls go through these clients on the same origin as the desk.
const transport = createConnectTransport({ baseUrl: window.location.origin })
export const systemClient = createClient(SystemService, transport)
export const entryClient = createClient(EntryService, transport)
const recordClient = createClient(RecordService, transport)

function fromMessage(record: RecordMessage): RecordDefinition {
  return {
    name: record.key,
    label: record.label,
    description: record.description,
    revision: Number(record.revision),
    fields: record.fields.map((field) => ({ name: field.key, label: field.label, type: field.type, required: field.required, ...(field.targetRecordKey ? { target_record: field.targetRecordKey } : {}) })),
  }
}

/** Loads every saved Record definition. */
export async function listRecords(): Promise<RecordDefinition[]> {
  const res = await recordClient.listRecords({})
  return res.records.map(fromMessage)
}

function toMessage(record: RecordDefinition): RecordMessage {
  return create(RecordSchema, {
    key: record.name,
    label: record.label,
    description: record.description,
    fields: record.fields.map((field) => ({ key: field.name, label: field.label, type: field.type, required: field.required, targetRecordKey: field.type === 'Link' ? field.target_record ?? '' : '' })),
  })
}

/** The table migration a save would run, as reported by the server. */
export type SavePlan = { steps: MigrationStep[]; requiresConfirmation: boolean }

/** Asks the server what saving this Record would change, without changing anything. */
export async function planRecordSave(record: RecordDefinition): Promise<SavePlan> {
  const res = await recordClient.planRecordSave({ record: toMessage(record), expectedRevision: BigInt(record.revision) })
  return { steps: res.steps, requiresConfirmation: res.requiresConfirmation }
}

/** Answers to a save plan: confirmation of data loss and fill-in values by field key. */
export type SaveAnswers = { confirmDataLoss: boolean; backfills: { [key: string]: FieldValue } }

/**
 * Saves a Record and returns it at its new revision. Throws ConnectError with
 * code Aborted when someone else saved the Record first, and FailedPrecondition
 * when the plan needs answers that were not given.
 */
export async function saveRecord(record: RecordDefinition, answers?: SaveAnswers): Promise<RecordDefinition> {
  const res = await recordClient.saveRecord({ record: toMessage(record), expectedRevision: BigInt(record.revision), confirmDataLoss: answers?.confirmDataLoss ?? false, backfills: answers?.backfills ?? {} })
  if (!res.record) throw new ConnectError('the server returned no Record')
  return fromMessage(res.record)
}

/**
 * Deletes a saved Record, its table and its entries. Throws ConnectError with
 * code FailedPrecondition when other Records link to it.
 */
export async function deleteRecord(record: RecordDefinition): Promise<void> {
  await recordClient.deleteRecord({ key: record.name, expectedRevision: BigInt(record.revision), deleteEntries: true })
}
