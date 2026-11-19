import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { SystemService } from './gen/athena/v1/system_pb'
import { RecordService, type FieldValue } from './gen/athena/v1/records_pb'
import { RecordTypeService, RecordTypeSchema, type MigrationStep, type RecordType as RecordTypeMessage, type RecordTypeField as RecordTypeFieldMessage } from './gen/athena/v1/record_types_pb'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import type { Field, Kind, RecordType } from './types'

// All ConnectRPC calls go through these clients on the same origin as the desk.
const transport = createConnectTransport({ baseUrl: window.location.origin })
export const systemClient = createClient(SystemService, transport)
export const recordClient = createClient(RecordService, transport)
const recordTypeClient = createClient(RecordTypeService, transport)

function fieldFromMessage(field: RecordTypeFieldMessage): Field {
  // Only attributes that are set are copied, so a field reads the same as one built in the desk.
  return {
    name: field.key,
    label: field.label,
    type: field.type,
    required: field.required,
    ...(field.targetRecordTypeKey ? { target_record_type: field.targetRecordTypeKey } : {}),
    ...(field.unique ? { unique: true } : {}),
    ...(field.defaultValue ? { default: field.defaultValue } : {}),
    ...(field.min ? { min: field.min } : {}),
    ...(field.max ? { max: field.max } : {}),
    ...(field.pattern ? { pattern: field.pattern } : {}),
    ...(field.maxLength ? { max_length: field.maxLength } : {}),
    ...(field.readOnly ? { read_only: true } : {}),
    ...(field.setOnce ? { set_once: true } : {}),
    ...(field.noCopy ? { no_copy: true } : {}),
    ...(field.index ? { index: true } : {}),
    ...(field.enumValues.length > 0 ? { values: field.enumValues.map((choice) => ({ value: choice.value, label: choice.label })) } : {}),
    ...(field.currency ? { currency: field.currency } : {}),
    ...(field.uom ? { uom: field.uom } : {}),
    ...(field.scale !== undefined ? { scale: field.scale } : {}),
  }
}

function fromMessage(recordType: RecordTypeMessage): RecordType {
  return {
    name: recordType.key,
    label: recordType.label,
    description: recordType.description,
    revision: Number(recordType.revision),
    kind: (recordType.kind || 'master') as Kind,
    ...(recordType.class ? { class: recordType.class } : {}),
    ...(recordType.parentRecordTypeKey ? { parent_record_type: recordType.parentRecordTypeKey } : {}),
    fields: recordType.fields.map(fieldFromMessage),
  }
}

/** Loads every saved Record Type. */
export async function listRecordTypes(): Promise<RecordType[]> {
  const res = await recordTypeClient.listRecordTypes({})
  return res.recordTypes.map(fromMessage)
}

function toMessage(recordType: RecordType): RecordTypeMessage {
  return create(RecordTypeSchema, {
    key: recordType.name,
    label: recordType.label,
    description: recordType.description,
    kind: recordType.kind,
    class: recordType.kind === 'document' ? recordType.class ?? '' : '',
    parentRecordTypeKey: recordType.kind === 'child' ? recordType.parent_record_type ?? '' : '',
    fields: recordType.fields.map((field) => ({
      key: field.name,
      label: field.label,
      type: field.type,
      required: field.required,
      targetRecordTypeKey: field.type === 'Link' ? field.target_record_type ?? '' : '',
      unique: field.unique ?? false,
      defaultValue: field.default ?? '',
      min: field.min ?? '',
      max: field.max ?? '',
      pattern: field.pattern ?? '',
      maxLength: field.max_length ?? 0,
      readOnly: field.read_only ?? false,
      setOnce: field.set_once ?? false,
      noCopy: field.no_copy ?? false,
      index: field.index ?? false,
      enumValues: field.type === 'Enum' ? field.values ?? [] : [],
      currency: field.type === 'Money' ? field.currency ?? '' : '',
      uom: field.type === 'Quantity' ? field.uom ?? '' : '',
      ...(field.type === 'Money' || field.type === 'Quantity' ? { scale: field.scale } : {}),
    })),
  })
}

/** The table migration a save would run, as reported by the server. */
export type SavePlan = { steps: MigrationStep[]; requiresConfirmation: boolean }

/** Asks the server what saving this Record Type would change, without changing anything. */
export async function planRecordTypeSave(recordType: RecordType): Promise<SavePlan> {
  const res = await recordTypeClient.planRecordTypeSave({ recordType: toMessage(recordType), expectedRevision: BigInt(recordType.revision) })
  return { steps: res.steps, requiresConfirmation: res.requiresConfirmation }
}

/** Answers to a save plan: confirmation of data loss and fill-in values by field key. */
export type SaveAnswers = { confirmDataLoss: boolean; backfills: { [key: string]: FieldValue } }

/**
 * Saves a Record Type and returns it at its new revision. Throws ConnectError
 * with code Aborted when someone else saved the Record Type first, and
 * FailedPrecondition when the plan needs answers that were not given.
 */
export async function saveRecordType(recordType: RecordType, answers?: SaveAnswers): Promise<RecordType> {
  const res = await recordTypeClient.saveRecordType({ recordType: toMessage(recordType), expectedRevision: BigInt(recordType.revision), confirmDataLoss: answers?.confirmDataLoss ?? false, backfills: answers?.backfills ?? {} })
  if (!res.recordType) throw new ConnectError('the server returned no Record Type')
  return fromMessage(res.recordType)
}

/**
 * Deletes a saved Record Type, its table and its Records. Throws ConnectError
 * with code FailedPrecondition when other Record Types link to it.
 */
export async function deleteRecordType(recordType: RecordType): Promise<void> {
  await recordTypeClient.deleteRecordType({ key: recordType.name, expectedRevision: BigInt(recordType.revision), deleteRecords: true })
}
