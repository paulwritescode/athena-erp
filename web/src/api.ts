import { createClient, type Interceptor } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { SystemService } from './gen/athena/v1/system_pb'
import { AuthService } from './gen/athena/v1/auth_pb'
import { FileService } from './gen/athena/v1/files_pb'
import { RuleService } from './gen/athena/v1/rules_pb'
import { LedgerService } from './gen/athena/v1/ledger_pb'
import { RecordService, type FieldValue } from './gen/athena/v1/records_pb'
import { RecordTypeService, RecordTypeSchema, type MigrationStep, type RecordType as RecordTypeMessage, type RecordTypeField as RecordTypeFieldMessage } from './gen/athena/v1/record_types_pb'
import { NotificationService } from './gen/athena/v1/notifications_pb'
import { WorkspaceService } from './gen/athena/v1/workspace_pb'
import { EventService, JobService } from './gen/athena/v1/events_pb'
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { isStepUpRequired, sessionEndReason } from './apiErrors'
import { layoutFromMessage, layoutToMessage } from './forms/layout'
import type { Field, Kind, RecordType } from './types'

// The session token lives in an HttpOnly cookie the browser sends by itself.
// The CSRF token comes from the sign-in or session response and goes in a
// header on every call, which a page on another site cannot do.
let csrfToken = ''

/** Sets the CSRF token sent with every call; '' after signing out. */
export function setCsrfToken(token: string) {
  csrfToken = token
}

let sessionEnded: (reason?: string) => void = () => {}

/** Registers what happens when the server says the session has ended. */
export function onSessionEnded(callback: (reason?: string) => void) {
  sessionEnded = callback
}

/** Lets one module end the session with a reason (a revoked step-up). */
export function emitSessionEnded(reason?: string) {
  sessionEnded(reason)
}

let stepUpAsker: (() => Promise<string | null>) | null = null

/** Registers the step-up password prompt (stepUp.ts owns the dialog). */
export function setStepUpAsker(ask: () => Promise<string | null>) {
  stepUpAsker = ask
}

const steppedUp = new WeakSet<object>()

type Procedure = { service: { typeName: string }; method: { name: string } }

function isAuthProcedure(req: Procedure): boolean {
  return req.service.typeName === 'athena.v1.AuthService' && (req.method.name === 'Login' || req.method.name === 'StepUp' || req.method.name === 'CompleteChallenge')
}

/**
 * Whether a failed call ends the session: a signed-in desk whose call came
 * back Unauthenticated, except the authentication calls themselves (a wrong
 * password at sign-in or step-up must never sign the desk out).
 */
export function shouldHandleSessionEnd(req: Procedure, err: unknown): boolean {
  return csrfToken !== '' && err instanceof ConnectError && err.code === Code.Unauthenticated && !isAuthProcedure(req)
}

function isStepUpable(req: Procedure): boolean {
  // Never intercept the authentication calls themselves.
  return !isAuthProcedure(req)
}

const csrfInterceptor: Interceptor = (next) => async (req) => {
  if (csrfToken) req.header.set('X-CSRF-Token', csrfToken)
  try {
    return await next(req)
  } catch (err) {
    // A failed sign-in is also Unauthenticated; only a signed-in desk reacts.
    if (shouldHandleSessionEnd(req, err)) {
      sessionEnded(sessionEndReason(err) ?? undefined)
      throw err
    }
    // Step-up: ask once, then retry the original call once. The dialog owns
    // the StepUp call itself (and its wrong-password retries); cancelling
    // reports Canceled, which reads as "Not changed." instead of an error.
    if (csrfToken && stepUpAsker && !steppedUp.has(req) && isStepUpable(req) && isStepUpRequired(err)) {
      steppedUp.add(req)
      const password = await stepUpAsker()
      if (password === null) throw new ConnectError('step-up cancelled', Code.Canceled)
      // Step-up rotates the session, so the retry carries the fresh token.
      if (csrfToken) req.header.set('X-CSRF-Token', csrfToken)
      return await next(req)
    }
    throw err
  }
}

// All ConnectRPC calls go through these clients on the same origin as the desk.
const transport = createConnectTransport({ baseUrl: window.location.origin, interceptors: [csrfInterceptor] })
export const systemClient = createClient(SystemService, transport)
export const authClient = createClient(AuthService, transport)
export const recordClient = createClient(RecordService, transport)
export const fileClient = createClient(FileService, transport)
export const ruleClient = createClient(RuleService, transport)
export const ledgerClient = createClient(LedgerService, transport)
const recordTypeClient = createClient(RecordTypeService, transport)
export const notificationClient = createClient(NotificationService, transport)
export const workspaceClient = createClient(WorkspaceService, transport)

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
    ...(field.allowOnSubmit ? { allow_on_submit: true } : {}),
    ...(field.index ? { index: true } : {}),
    ...(field.enumValues.length > 0 ? { values: field.enumValues.map((choice) => ({ value: choice.value, label: choice.label })) } : {}),
    ...(field.currency ? { currency: field.currency } : {}),
    ...(field.uom ? { uom: field.uom } : {}),
    ...(field.scale !== undefined ? { scale: field.scale } : {}),
    ...(field.formula ? { formula: field.formula } : {}),
    ...(field.requiredIf ? { required_if: field.requiredIf } : {}),
    ...(field.readOnlyIf ? { read_only_if: field.readOnlyIf } : {}),
    ...(field.validate ? { validate: field.validate } : {}),
    ...(field.dependsOn ? { depends_on: field.dependsOn } : {}),
    ...(field.permissionLevel ? { permission_level: field.permissionLevel } : {}),
  }
}

function fromMessage(recordType: RecordTypeMessage): RecordType {
  const layout = layoutFromMessage(recordType.layout)
  return {
    name: recordType.key,
    label: recordType.label,
    description: recordType.description,
    revision: Number(recordType.revision),
    kind: (recordType.kind || 'master') as Kind,
    ...(recordType.class ? { class: recordType.class } : {}),
    ...(recordType.parentRecordTypeKey ? { parent_record_type: recordType.parentRecordTypeKey } : {}),
    ...(recordType.submittable ? { submittable: true } : {}),
    ...(recordType.amendable ? { amendable: true } : {}),
    ...(recordType.namingSeries ? { naming_series: recordType.namingSeries } : {}),
    ...(recordType.gapFree ? { gap_free: true } : {}),
    ...(recordType.allowManualName ? { allow_manual_name: true } : {}),
    ...(recordType.tree ? { tree: true } : {}),
    ...(recordType.effectiveDated ? { effective_dated: true } : {}),
    ...(recordType.labelField ? { label_field: recordType.labelField } : {}),
    fields: recordType.fields.map(fieldFromMessage),
    ...(layout ? { layout } : {}),
    ...(recordType.access ? { access: { actions: recordType.access.actions, permission_level: recordType.access.permissionLevel } } : {}),
    record_count: Number(recordType.recordCount),
    ...(recordType.countHidden ? { count_hidden: true } : {}),
    ...(recordType.updatedAt ? { updated_at: recordType.updatedAt } : {}),
    ...(recordType.updatedBy ? { updated_by: recordType.updatedBy } : {}),
    ...(recordType.packKey ? { pack_key: recordType.packKey } : {}),
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
    revision: BigInt(recordType.revision),
    kind: recordType.kind,
    class: recordType.kind === 'document' ? recordType.class ?? '' : '',
    parentRecordTypeKey: recordType.kind === 'child' ? recordType.parent_record_type ?? '' : '',
    submittable: recordType.kind === 'document' && (recordType.submittable ?? false),
    amendable: recordType.kind === 'document' && (recordType.submittable ?? false) && (recordType.amendable ?? false),
    namingSeries: recordType.kind === 'document' && (recordType.submittable ?? false) ? recordType.naming_series ?? '' : '',
    gapFree: recordType.kind === 'document' && (recordType.submittable ?? false) && (recordType.naming_series ?? '') !== '' && (recordType.gap_free ?? false),
    allowManualName: recordType.kind === 'document' && (recordType.submittable ?? false) && (recordType.allow_manual_name ?? false),
    tree: (recordType.kind === 'master' || recordType.kind === 'document') && (recordType.tree ?? false),
    effectiveDated: recordType.effective_dated ?? false,
    labelField: recordType.label_field ?? '',
    layout: layoutToMessage(recordType.layout),
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
      allowOnSubmit: field.allow_on_submit ?? false,
      index: field.index ?? false,
      enumValues: field.type === 'Enum' ? field.values ?? [] : [],
      currency: field.type === 'Money' ? field.currency ?? '' : '',
      uom: field.type === 'Quantity' ? field.uom ?? '' : '',
      ...(field.type === 'Money' || field.type === 'Quantity' ? { scale: field.scale } : {}),
      formula: field.formula ?? '',
      requiredIf: field.required_if ?? '',
      readOnlyIf: field.read_only_if ?? '',
      validate: field.validate ?? '',
      dependsOn: field.depends_on ?? '',
      permissionLevel: field.permission_level ?? 0,
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
 * Saves a Record Type and returns it at its new revision. A child Record Type
 * may bring its saved parent along, so a parent rollup over the child's
 * fields is saved in the same request. Throws ConnectError
 * with code Aborted when someone else saved the Record Type first, and
 * FailedPrecondition when the plan needs answers that were not given.
 */
export async function saveRecordType(recordType: RecordType, answers?: SaveAnswers, parent?: RecordType): Promise<{ saved: RecordType; parent?: RecordType }> {
  const res = await recordTypeClient.saveRecordType({ recordType: toMessage(recordType), expectedRevision: BigInt(recordType.revision), confirmDataLoss: answers?.confirmDataLoss ?? false, backfills: answers?.backfills ?? {}, ...(parent ? { parent: toMessage(parent) } : {}) })
  if (!res.recordType) throw new ConnectError('the server returned no Record Type')
  return { saved: fromMessage(res.recordType), ...(res.parent ? { parent: fromMessage(res.parent) } : {}) }
}

/**
 * Deletes a saved Record Type, its table and its Records. Throws ConnectError
 * with code FailedPrecondition when other Record Types link to it.
 */
export async function deleteRecordType(recordType: RecordType): Promise<void> {
  await recordTypeClient.deleteRecordType({ key: recordType.name, expectedRevision: BigInt(recordType.revision), deleteRecords: true })
}

// Durable background work (ADR-0028, Proposed): the event bus and the job
// runner behind the build permission.
export const eventClient = createClient(EventService, transport)
export const jobClient = createClient(JobService, transport)
