// The everyday-words glossary (B6, design-system section 8, R6.1): one entry
// per term a user meets, with the label to show and a one-line plain
// explanation. View mode renders its labels, statuses and empty states only
// through gloss(); Build mode keeps builder words and explains them with
// HelpHint. Copy rules: sentences, not title case; errors say what happened
// and what to do next.
export type GlossaryEntry = {
  /** The words the user sees. */
  label: string
  /** One plain sentence saying what the term means. */
  explanation: string
}

export const glossary = {
  // Lifecycle: the states a document moves through. Only the server moves a
  // document between these states.
  draft: { label: 'Draft', explanation: 'Still being worked on. It can be edited and deleted.' },
  submitted: { label: 'Submitted', explanation: 'Finished and sent on. Only specially allowed fields can change now.' },
  cancelled: { label: 'Cancelled', explanation: 'Stopped for good. It stays visible but cannot be edited.' },
  amended: { label: 'Amended', explanation: 'A cancelled record was corrected by making a new draft from it.' },
  posted: { label: 'Recorded in the books', explanation: 'Saved to the accounts, so reports include it.' },
  // Records: what a user keeps and what defines it.
  record: { label: 'Record', explanation: 'One saved entry, such as one invoice.' },
  recordType: { label: 'Record Type', explanation: 'A definition of one kind of record: its fields and its rules.' },
  recordKind: { label: 'Kind', explanation: 'What sort of thing a form keeps: a list, a document, lines, or a setting.' },
  kindMaster: { label: 'Master', explanation: 'A list the workspace keeps, such as customers or items.' },
  kindDocument: { label: 'Document', explanation: 'Work that moves from draft to submitted, such as an invoice.' },
  kindChild: { label: 'Child', explanation: 'Lines that belong to another record, such as invoice lines.' },
  kindLedger: { label: 'Ledger', explanation: "The system's own record of money movements, never edited by hand." },
  kindBalance: { label: 'Balance', explanation: 'A stored total rebuilt from the ledger.' },
  kindSetting: { label: 'Setting', explanation: 'A single row of choices for the whole workspace.' },
  kindLog: { label: 'Log', explanation: 'History the system writes itself, never edited by hand.' },
  kindSnapshot: { label: 'Snapshot', explanation: 'A frozen copy of data from one moment.' },
  kindMeta: { label: 'Meta', explanation: 'Information about the system itself.' },
  // Ledger: companies, periods, money and accounts.
  company: { label: 'Company', explanation: 'One business whose books are kept separately.' },
  financialYear: { label: 'Financial year', explanation: 'The twelve months the books are reported over.' },
  period: { label: 'Period', explanation: 'One slice of the financial year, usually a month, that opens and closes.' },
  softClose: { label: 'Soft close', explanation: 'No new entries can be added, but corrections can still be posted.' },
  hardClose: { label: 'Hard close', explanation: 'Nothing can be recorded or changed in this period again, ever.' },
  currency: { label: 'Currency', explanation: 'The money a company or an entry is counted in.' },
  exchangeRate: { label: 'Exchange rate', explanation: 'What one money is worth in another, from a moment on.' },
  account: { label: 'Account', explanation: 'One named place where money movements are recorded.' },
  accountType: { label: 'Account type', explanation: 'What kind of place an account is: asset, liability, income, cost or equity.' },
  posting: { label: 'Books entry', explanation: 'One movement recorded in the accounts.' },
  // Access: people, what they may do, and how they sign in.
  role: { label: 'Role', explanation: 'A named set of things a person is allowed to do.' },
  grant: { label: 'Grant', explanation: 'One allowed action on one kind of record.' },
  assignment: { label: 'Assignment', explanation: 'Giving a role to one person, with limits and dates.' },
  scope: { label: 'Scope', explanation: 'Which companies or warehouses an assignment reaches.' },
  session: { label: 'Session', explanation: 'One signed-in device. Ending it signs that device out.' },
  stepUp: { label: 'Confirm it is you', explanation: 'Entering your password again before a sensitive change.' },
  invite: { label: 'Invite', explanation: 'A one-time code that lets a new person sign in for the first time.' },
  owner: { label: 'Owner', explanation: 'The person who can make others owners and approve the biggest changes.' },
  rootAdministrator: { label: 'Root administrator', explanation: 'The built-in account that can do everything and cannot be changed or removed.' },
  // Rules: automatic checks and fill-ins set up by a builder.
  rule: { label: 'Rule', explanation: 'A small automatic check or fill-in set up by a builder.' },
  validation: { label: 'Validation', explanation: 'A check that stops a save and says what is wrong.' },
  derivation: { label: 'Derivation', explanation: 'A rule that fills a value in from the other fields.' },
  guard: { label: 'Guard', explanation: 'A rule that allows or blocks moving to the next step.' },
  transform: { label: 'Transform', explanation: 'A rule that reshapes values when asked, changing nothing stored.' },
  // Packs and workspaces.
  pack: { label: 'Pack', explanation: 'A bundle of forms and rules that installs together.' },
  workspace: { label: 'Workspace', explanation: "This company's own space: its data, people and settings." },
  // Notifications: the kinds the inbox can hold.
  inviteAccepted: { label: 'Invite accepted', explanation: 'Someone you invited signed in for the first time.' },
  accessChanged: { label: 'Access changed', explanation: 'Your roles or limits changed, so what you can see may differ.' },
  signInFailed: { label: 'Failed sign-in', explanation: 'Someone tried to sign in to the account and got it wrong.' },
  accountLocked: { label: 'Account locked', explanation: 'Too many failed sign-ins locked the account for a while.' },
  sessionEnded: { label: 'Session ended', explanation: 'One of the sign-ins was ended by someone else.' },
  saveStopped: { label: 'Save stopped', explanation: 'An automatic check stopped a save. Fix what it names and try again.' },
  periodClosed: { label: 'Period closed', explanation: 'A month worked in was closed for changes.' },
  approvalWaiting: { label: 'Approval waiting', explanation: 'Something needs approval before it can continue.' },
  jobFinished: { label: 'Job finished', explanation: 'Background work started earlier is done.' },
  jobFailed: { label: 'Job failed', explanation: 'Background work started earlier could not finish.' },
  // View-mode record details.
  history: { label: 'History', explanation: 'Every change to this record, who made it and when.' },
  formVersion: { label: 'Form version', explanation: 'Which version of the form this record was saved with.' },
  // Fields: the words on the field settings sheet.
  unique: { label: 'Unique', explanation: 'No two records share a value. A deleted master keeps its value.' },
  indexed: { label: 'Indexed', explanation: 'The workspace keeps a lookup so finding by this field is faster.' },
  readOnly: { label: 'Read only', explanation: 'People cannot change the value; only the default sets it.' },
  setOnce: { label: 'Set once', explanation: 'Cannot change after it has a value.' },
  noCopy: { label: 'Do not copy', explanation: 'Left empty when a document is amended.' },
  allowOnSubmit: { label: 'Allow on submit', explanation: 'Still editable after the document is submitted. Every change is audited.' },
  permissionLevel: { label: 'Permission level', explanation: 'Only people whose access reaches this level see or change the field. Level 0 is everyone.' },
  fieldFormula: { label: 'Formula', explanation: 'Calculated by the server on every save from the other fields. People cannot edit it.' },
  fieldPattern: { label: 'Pattern', explanation: 'A regular expression the whole value must match.' },
  linkTarget: { label: 'Links to', explanation: 'The kind of record this field points to.' },
  fieldChoices: { label: 'Choices', explanation: 'Fixed options people pick from; a value is permanent once saved.' },
} satisfies Record<string, GlossaryEntry>

export type GlossaryKey = keyof typeof glossary

export function gloss(key: GlossaryKey): GlossaryEntry {
  return glossary[key]
}

export function glossaryKeys(): GlossaryKey[] {
  return Object.keys(glossary) as GlossaryKey[]
}
