<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { ledgerClient } from '../api'
import ActionButton from '@/components/ActionButton.vue'
import HelpHint from '@/copy/HelpHint.vue'
import ActionGroup from '@/components/ActionGroup.vue'
import type { ActionItem } from '@/components/actions'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type TableFilter } from '@/components/DataTable.vue'
import DetailSheet from '@/components/DetailSheet.vue'
import PrimaryCell from '@/components/PrimaryCell.vue'
import CheckboxField from '@/components/fields/CheckboxField.vue'
import DateField from '@/components/fields/DateField.vue'
import NumberField from '@/components/fields/NumberField.vue'
import SelectField from '@/components/fields/SelectField.vue'
import TextField from '@/components/fields/TextField.vue'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type {
  Account,
  AccountType,
  Branch,
  Company,
  CostCentre,
  Currency,
  Department,
  Determination,
  DimensionDefinition,
  FiscalYear,
  Period,
  ReadinessCheck,
} from '@/gen/athena/v1/ledger_pb'

const props = withDefaults(defineProps<{ canWrite: boolean; username?: string }>(), { username: '' })

function failure(err: unknown, action: string): void {
  toast.error(err instanceof ConnectError ? `${action}: ${err.rawMessage}` : `${action}.`)
}

// Master cells share a few shapes: the primary code in mono with its
// label below it, muted dashes for empties, lowercase yes/no flags.
function primaryCode(title: string, subtitle: string) {
  return h(PrimaryCell, { title, subtitle: subtitle || null, mono: true })
}

function dashCell(value: string) {
  return value === '' ? h('span', { class: 'text-muted-foreground' }, '—') : value
}

// The company every scoped section works on.
const companies = ref<Company[]>([])
const currentCompany = ref('')

const companyForm = ref({ name: '', label: '', baseCurrency: '', disabled: false, revision: BigInt(0) })

async function loadCompanies(): Promise<void> {
  try {
    companies.value = (await ledgerClient.listCompanies({})).companies
    if (!companies.value.some((item) => item.name === currentCompany.value)) {
      currentCompany.value = companies.value[0]?.name ?? ''
    }
  } catch (err) {
    failure(err, 'Could not load companies')
  }
}

function editCompany(company: Company): void {
  companyForm.value = { name: company.name, label: company.label, baseCurrency: company.baseCurrency, disabled: company.disabled, revision: company.revision }
  currentCompany.value = company.name
}

function resetCompanyForm(): void {
  companyForm.value = { name: '', label: '', baseCurrency: '', disabled: false, revision: BigInt(0) }
}

// The §16 company sheet: rows open it to view and edit, New company opens
// it blank. Closing with edits asks first.
const companyOpen = ref(false)
const companyCreating = ref(false)
const companySnapshot = ref('')
const discardingCompany = ref(false)

function snapshotCompany(): string {
  const form = companyForm.value
  return JSON.stringify({ name: form.name, label: form.label, baseCurrency: form.baseCurrency, disabled: form.disabled })
}
const companyDirty = computed(() => companyOpen.value && snapshotCompany() !== companySnapshot.value)

function openCompany(company?: Company): void {
  if (company) {
    editCompany(company)
    companyCreating.value = false
  } else {
    resetCompanyForm()
    companyCreating.value = true
  }
  companySnapshot.value = snapshotCompany()
  companyOpen.value = true
}

function requestCompanyClose(): void {
  if (companyDirty.value) discardingCompany.value = true
  else companyOpen.value = false
}

async function saveCompany(): Promise<void> {
  const form = companyForm.value
  try {
    const res = await ledgerClient.saveCompany({
      company: { name: form.name, label: form.label, baseCurrency: form.baseCurrency, disabled: form.disabled },
      expectedRevision: form.revision,
    })
    toast.success(`Saved company ${res.company?.name}.`)
    resetCompanyForm()
    companyOpen.value = false
    await loadCompanies()
    if (res.company) currentCompany.value = res.company.name
  } catch (err) {
    failure(err, 'Could not save the company')
  }
}

const companySearch = ref('')
const companyFilteredActive = computed(() => companySearch.value.trim() !== '')
const filteredCompanies = computed(() => {
  const needle = companySearch.value.trim().toLowerCase()
  return companies.value.filter((company) => !needle || `${company.name} ${company.label}`.toLowerCase().includes(needle))
})
const companyColumns = [
  {
    id: 'code', header: 'Code', accessorFn: (company: Company) => company.name.toLowerCase(), meta: { label: 'Code' },
    cell: ({ row }: { row: { original: Company } }) => primaryCode(row.original.name, row.original.label),
  },
  {
    id: 'money', header: 'Main money', accessorFn: (company: Company) => company.baseCurrency, meta: { label: 'Main money' },
    cell: ({ row }: { row: { original: Company } }) => dashCell(row.original.baseCurrency),
  },
]

// Branches, departments and cost centres of the current company.
const branches = ref<Branch[]>([])
const departments = ref<Department[]>([])
const costCentres = ref<CostCentre[]>([])
const branchForm = ref({ name: '', label: '', parent: '', isGroup: false, revision: BigInt(0) })
const departmentForm = ref({ name: '', label: '', branch: '', parent: '', isGroup: false, revision: BigInt(0) })
const costCentreForm = ref({ name: '', label: '', parent: '', isGroup: false, revision: BigInt(0) })

async function loadOrg(): Promise<void> {
  const company = currentCompany.value
  if (!company) {
    branches.value = []
    departments.value = []
    costCentres.value = []
    return
  }
  try {
    const [b, d, c] = await Promise.all([
      ledgerClient.listBranches({ company }),
      ledgerClient.listDepartments({ company }),
      ledgerClient.listCostCentres({ company }),
    ])
    if (currentCompany.value !== company) return
    branches.value = b.branches
    departments.value = d.departments
    costCentres.value = c.costCentres
  } catch (err) {
    if (currentCompany.value === company) failure(err, 'Could not load organisation units')
  }
}

async function saveBranch(): Promise<void> {
  const form = branchForm.value
  try {
    await ledgerClient.saveBranch({
      branch: { name: form.name, label: form.label, company: currentCompany.value, parent: form.parent, isGroup: form.isGroup },
      expectedRevision: form.revision,
    })
    toast.success(`Saved branch ${form.name}.`)
    branchForm.value = { name: '', label: '', parent: '', isGroup: false, revision: BigInt(0) }
    await loadOrg()
  } catch (err) {
    failure(err, 'Could not save the branch')
  }
}

function editBranch(branch: Branch): void {
  branchForm.value = { name: branch.name, label: branch.label, parent: branch.parent, isGroup: branch.isGroup, revision: branch.revision }
}

async function saveDepartment(): Promise<void> {
  const form = departmentForm.value
  try {
    await ledgerClient.saveDepartment({
      department: { name: form.name, label: form.label, company: currentCompany.value, branch: form.branch, parent: form.parent, isGroup: form.isGroup },
      expectedRevision: form.revision,
    })
    toast.success(`Saved department ${form.name}.`)
    departmentForm.value = { name: '', label: '', branch: '', parent: '', isGroup: false, revision: BigInt(0) }
    await loadOrg()
  } catch (err) {
    failure(err, 'Could not save the department')
  }
}

function editDepartment(department: Department): void {
  departmentForm.value = { name: department.name, label: department.label, branch: department.branch, parent: department.parent, isGroup: department.isGroup, revision: department.revision }
}

async function saveCostCentre(): Promise<void> {
  const form = costCentreForm.value
  try {
    await ledgerClient.saveCostCentre({
      costCentre: { name: form.name, label: form.label, company: currentCompany.value, parent: form.parent, isGroup: form.isGroup },
      expectedRevision: form.revision,
    })
    toast.success(`Saved cost centre ${form.name}.`)
    costCentreForm.value = { name: '', label: '', parent: '', isGroup: false, revision: BigInt(0) }
    await loadOrg()
  } catch (err) {
    failure(err, 'Could not save the cost centre')
  }
}

function editCostCentre(centre: CostCentre): void {
  costCentreForm.value = { name: centre.name, label: centre.label, parent: centre.parent, isGroup: centre.isGroup, revision: centre.revision }
}

const branchSearch = ref('')
const branchFilteredActive = computed(() => branchSearch.value.trim() !== '')
const filteredBranches = computed(() => {
  const needle = branchSearch.value.trim().toLowerCase()
  return branches.value.filter((branch) => !needle || `${branch.name} ${branch.label}`.toLowerCase().includes(needle))
})
const branchColumns = [
  {
    id: 'code', header: 'Code', accessorFn: (branch: Branch) => branch.name.toLowerCase(), meta: { label: 'Code' },
    cell: ({ row }: { row: { original: Branch } }) => primaryCode(row.original.name, row.original.label),
  },
  {
    id: 'parent', header: 'Belongs under', accessorFn: (branch: Branch) => branch.parent, meta: { label: 'Belongs under' },
    cell: ({ row }: { row: { original: Branch } }) => dashCell(row.original.parent),
  },
  {
    id: 'group', header: 'Group', accessorFn: (branch: Branch) => (branch.isGroup ? 'yes' : 'no'), meta: { label: 'Group' },
    cell: ({ row }: { row: { original: Branch } }) => (row.original.isGroup ? 'yes' : 'no'),
  },
]

const departmentSearch = ref('')
const departmentFilteredActive = computed(() => departmentSearch.value.trim() !== '')
const filteredDepartments = computed(() => {
  const needle = departmentSearch.value.trim().toLowerCase()
  return departments.value.filter((department) => !needle || `${department.name} ${department.label} ${department.branch}`.toLowerCase().includes(needle))
})
const departmentColumns = [
  {
    id: 'code', header: 'Code', accessorFn: (department: Department) => department.name.toLowerCase(), meta: { label: 'Code' },
    cell: ({ row }: { row: { original: Department } }) => primaryCode(row.original.name, row.original.label),
  },
  {
    id: 'branch', header: 'Branch', accessorFn: (department: Department) => department.branch, meta: { label: 'Branch' },
    cell: ({ row }: { row: { original: Department } }) => dashCell(row.original.branch),
  },
  {
    id: 'parent', header: 'Belongs under', accessorFn: (department: Department) => department.parent, meta: { label: 'Belongs under' },
    cell: ({ row }: { row: { original: Department } }) => dashCell(row.original.parent),
  },
]

const costCentreSearch = ref('')
const costCentreFilteredActive = computed(() => costCentreSearch.value.trim() !== '')
const filteredCostCentres = computed(() => {
  const needle = costCentreSearch.value.trim().toLowerCase()
  return costCentres.value.filter((centre) => !needle || `${centre.name} ${centre.label}`.toLowerCase().includes(needle))
})
const costCentreColumns = [
  {
    id: 'code', header: 'Code', accessorFn: (centre: CostCentre) => centre.name.toLowerCase(), meta: { label: 'Code' },
    cell: ({ row }: { row: { original: CostCentre } }) => primaryCode(row.original.name, row.original.label),
  },
  {
    id: 'parent', header: 'Belongs under', accessorFn: (centre: CostCentre) => centre.parent, meta: { label: 'Belongs under' },
    cell: ({ row }: { row: { original: CostCentre } }) => dashCell(row.original.parent),
  },
  {
    id: 'group', header: 'Group', accessorFn: (centre: CostCentre) => (centre.isGroup ? 'yes' : 'no'), meta: { label: 'Group' },
    cell: ({ row }: { row: { original: CostCentre } }) => (row.original.isGroup ? 'yes' : 'no'),
  },
]

// Dimensions.
const dimensions = ref<DimensionDefinition[]>([])
const dimensionForm = ref({ name: '', label: '', members: '', mandatoryFor: '', validFrom: '', validTo: '', revision: BigInt(0) })

function splitList(text: string): string[] {
  return text.split(',').map((item) => item.trim()).filter((item) => item !== '')
}

async function loadDimensions(): Promise<void> {
  try {
    dimensions.value = (await ledgerClient.listDimensions({})).dimensions
  } catch (err) {
    failure(err, 'Could not load dimensions')
  }
}

async function saveDimension(): Promise<void> {
  const form = dimensionForm.value
  try {
    await ledgerClient.saveDimension({
      dimension: { name: form.name, label: form.label, members: splitList(form.members), mandatoryFor: splitList(form.mandatoryFor), validFrom: form.validFrom, validTo: form.validTo },
      expectedRevision: form.revision,
    })
    toast.success(`Saved dimension ${form.name}.`)
    dimensionForm.value = { name: '', label: '', members: '', mandatoryFor: '', validFrom: '', validTo: '', revision: BigInt(0) }
    await loadDimensions()
  } catch (err) {
    failure(err, 'Could not save the dimension')
  }
}

function editDimension(dimension: DimensionDefinition): void {
  dimensionForm.value = { name: dimension.name, label: dimension.label, members: dimension.members.join(', '), mandatoryFor: dimension.mandatoryFor.join(', '), validFrom: dimension.validFrom, validTo: dimension.validTo, revision: dimension.revision }
}

const dimensionSearch = ref('')
const dimensionFilteredActive = computed(() => dimensionSearch.value.trim() !== '')
const filteredDimensions = computed(() => {
  const needle = dimensionSearch.value.trim().toLowerCase()
  return dimensions.value.filter((dimension) => !needle || `${dimension.name} ${dimension.label}`.toLowerCase().includes(needle))
})
const dimensionColumns = [
  {
    id: 'code', header: 'Code', accessorFn: (dimension: DimensionDefinition) => dimension.name.toLowerCase(), meta: { label: 'Code' },
    cell: ({ row }: { row: { original: DimensionDefinition } }) => primaryCode(row.original.name, row.original.label),
  },
  {
    id: 'members', header: 'Allowed values', accessorFn: (dimension: DimensionDefinition) => dimension.members.join(', '), meta: { label: 'Allowed values' },
    cell: ({ row }: { row: { original: DimensionDefinition } }) => row.original.members.join(', '),
  },
  {
    id: 'mandatory', header: 'Always needed for', accessorFn: (dimension: DimensionDefinition) => dimension.mandatoryFor.join(', '), meta: { label: 'Always needed for' },
    cell: ({ row }: { row: { original: DimensionDefinition } }) => dashCell(row.original.mandatoryFor.join(', ')),
  },
]

// Fiscal years and periods of the current company.
const fiscalYears = ref<FiscalYear[]>([])
const periods = ref<Period[]>([])
const book = ref('main')
const fiscalYearForm = ref({ name: '', label: '', startDate: '', endDate: '', revision: BigInt(0) })
const periodForm = ref({ name: '', label: '', fiscalYear: '', startDate: '', endDate: '', revision: BigInt(0) })

async function loadPeriods(): Promise<void> {
  const company = currentCompany.value
  if (!company) {
    fiscalYears.value = []
    periods.value = []
    return
  }
  try {
    const [years, list] = await Promise.all([
      ledgerClient.listFiscalYears({ company }),
      ledgerClient.listPeriods({ company, book: book.value || 'main' }),
    ])
    if (currentCompany.value !== company) return
    fiscalYears.value = years.fiscalYears
    periods.value = list.periods
  } catch (err) {
    if (currentCompany.value === company) failure(err, 'Could not load fiscal years and periods')
  }
}

async function saveFiscalYear(): Promise<void> {
  const form = fiscalYearForm.value
  try {
    await ledgerClient.saveFiscalYear({
      fiscalYear: { name: form.name, label: form.label, company: currentCompany.value, startDate: form.startDate, endDate: form.endDate },
      expectedRevision: form.revision,
    })
    toast.success(`Saved financial year ${form.name}.`)
    fiscalYearForm.value = { name: '', label: '', startDate: '', endDate: '', revision: BigInt(0) }
    await loadPeriods()
  } catch (err) {
    failure(err, 'Could not save the fiscal year')
  }
}

function editFiscalYear(year: FiscalYear): void {
  fiscalYearForm.value = { name: year.name, label: year.label, startDate: year.startDate, endDate: year.endDate, revision: year.revision }
}

const fiscalYearSearch = ref('')
const fiscalYearFilteredActive = computed(() => fiscalYearSearch.value.trim() !== '')
const filteredFiscalYears = computed(() => {
  const needle = fiscalYearSearch.value.trim().toLowerCase()
  return fiscalYears.value.filter((year) => !needle || `${year.name} ${year.label}`.toLowerCase().includes(needle))
})
const fiscalYearColumns = [
  {
    id: 'code', header: 'Code', accessorFn: (year: FiscalYear) => year.name.toLowerCase(), meta: { label: 'Code' },
    cell: ({ row }: { row: { original: FiscalYear } }) => primaryCode(row.original.name, row.original.label),
  },
  {
    id: 'dates', header: 'Dates', accessorFn: (year: FiscalYear) => year.startDate, meta: { label: 'Dates' },
    cell: ({ row }: { row: { original: FiscalYear } }) => `${row.original.startDate}..${row.original.endDate}`,
  },
]

async function savePeriod(): Promise<void> {
  const form = periodForm.value
  try {
    await ledgerClient.savePeriod({
      period: { company: currentCompany.value, book: book.value || 'main', name: form.name, label: form.label, fiscalYear: form.fiscalYear, startDate: form.startDate, endDate: form.endDate },
      expectedRevision: form.revision,
    })
    toast.success(`Saved period ${form.name}.`)
    periodForm.value = { name: '', label: '', fiscalYear: '', startDate: '', endDate: '', revision: BigInt(0) }
    periodOpen.value = false
    await loadPeriods()
  } catch (err) {
    failure(err, 'Could not save the period')
  }
}

function editPeriod(period: Period): void {
  periodForm.value = { name: period.name, label: period.label, fiscalYear: period.fiscalYear, startDate: period.startDate, endDate: period.endDate, revision: period.revision }
}

// The §16 period sheet: rows open it to view and edit with the status
// actions in the footer, New period opens it blank.
const periodOpen = ref(false)
const periodCreating = ref(false)
const periodTarget = ref<Period | null>(null)
const periodSnapshot = ref('')
const discardingPeriod = ref(false)

function snapshotPeriod(): string {
  const form = periodForm.value
  return JSON.stringify({ name: form.name, label: form.label, fiscalYear: form.fiscalYear, startDate: form.startDate, endDate: form.endDate })
}
const periodDirty = computed(() => periodOpen.value && snapshotPeriod() !== periodSnapshot.value)

function openPeriod(period?: Period): void {
  if (period) {
    editPeriod(period)
    periodTarget.value = period
    periodCreating.value = false
  } else {
    periodForm.value = { name: '', label: '', fiscalYear: '', startDate: '', endDate: '', revision: BigInt(0) }
    periodTarget.value = null
    periodCreating.value = true
  }
  periodSnapshot.value = snapshotPeriod()
  periodOpen.value = true
}

function requestPeriodClose(): void {
  if (periodDirty.value) discardingPeriod.value = true
  else periodOpen.value = false
}

const periodActions = computed<ActionItem[]>(() => {
  const items: ActionItem[] = [{ id: 'cancel', label: 'Cancel', kind: 'secondary', icon: 'x' }]
  if (props.canWrite) items.push({ id: 'save', label: 'Save period', kind: 'primary', icon: 'save' })
  const status = periodTarget.value?.status
  if (props.canWrite && !periodCreating.value && status === 'open') {
    items.push({ id: 'close', label: 'Close', kind: 'caution', icon: 'lock' })
  }
  if (props.canWrite && !periodCreating.value && status === 'soft_closed') {
    items.push({ id: 'reopen', label: 'Reopen', kind: 'restore', icon: 'rotate-ccw' })
  }
  if (props.canWrite && !periodCreating.value && status && status !== 'hard_closed') {
    items.push({ id: 'hard', label: 'Hard close', kind: 'destructive', icon: 'lock' })
  }
  return items
})

function onPeriodAction(id: string): void {
  if (id === 'cancel') periodOpen.value = false
  else if (id === 'save') void savePeriod()
  else if (id === 'close' && periodTarget.value) void setPeriodStatus(periodTarget.value, 'soft_closed')
  else if (id === 'reopen' && periodTarget.value) void setPeriodStatus(periodTarget.value, 'open')
  else if (id === 'hard' && periodTarget.value) hardClosing.value = periodTarget.value
}

async function setPeriodStatus(period: Period, status: string): Promise<void> {
  try {
    await ledgerClient.setPeriodStatus({ company: period.company, book: period.book, name: period.name, status, expectedRevision: period.revision })
    toast.success(`Period ${period.name} is now ${status}.`)
    await loadPeriods()
    // A status change moves the revision: sync an open sheet so a later
    // save does not trip on the stale one.
    if (periodOpen.value && periodTarget.value?.name === period.name) {
      const fresh = periods.value.find((item) => item.name === period.name) ?? null
      periodTarget.value = fresh
      if (fresh) periodForm.value.revision = fresh.revision
    }
  } catch (err) {
    failure(err, 'Could not change the period status')
  }
}

const periodSearch = ref('')
const periodStatusSelected = ref<string[]>([])
const periodFilters = computed<TableFilter[]>(() => {
  const statuses = [...new Set(periods.value.map((period) => period.status))].sort()
  return [{ id: 'status', label: 'Status', options: statuses.map((status) => ({ value: status, label: status })), selected: periodStatusSelected.value }]
})
const periodFilteredActive = computed(() => periodSearch.value.trim() !== '' || periodStatusSelected.value.length > 0)
const filteredPeriods = computed(() => {
  const needle = periodSearch.value.trim().toLowerCase()
  return periods.value.filter((period) => {
    if (periodStatusSelected.value.length > 0 && !periodStatusSelected.value.includes(period.status)) return false
    return !needle || `${period.name} ${period.label}`.toLowerCase().includes(needle)
  })
})
function onPeriodFiltersChanged(next: TableFilter[]) {
  periodStatusSelected.value = next.find((filter) => filter.id === 'status')?.selected ?? []
}
const periodColumns = [
  {
    id: 'code', header: 'Code', accessorFn: (period: Period) => period.name.toLowerCase(), meta: { label: 'Code' },
    cell: ({ row }: { row: { original: Period } }) => primaryCode(row.original.name, row.original.label),
  },
  {
    id: 'dates', header: 'Dates', accessorFn: (period: Period) => period.startDate, meta: { label: 'Dates' },
    cell: ({ row }: { row: { original: Period } }) => `${row.original.startDate}..${row.original.endDate}`,
  },
  {
    id: 'status', header: 'Status', accessorFn: (period: Period) => period.status, meta: { label: 'Status' },
    cell: ({ row }: { row: { original: Period } }) => h(Badge, { variant: row.original.status === 'open' ? 'secondary' : 'outline' }, () => row.original.status),
  },
]

// A hard close is permanent, so it asks first and names the consequence.
// The server exposes no step-up challenge for this call (SetPeriodStatus
// takes no proof), so the destructive confirm is the gate.
const hardClosing = ref<Period | null>(null)

function confirmHardClose() {
  const period = hardClosing.value
  hardClosing.value = null
  if (period) void setPeriodStatus(period, 'hard_closed')
}

// Currencies and exchange rates.
const currencies = ref<Currency[]>([])
const currencyForm = ref({ code: '', label: '', exponent: 2, disabled: false, revision: BigInt(0) })
const rateForm = ref({ from: '', to: '', rateType: 'spot', source: '', at: '', rate: '' })
const lookedUpRate = ref('')

async function loadCurrencies(): Promise<void> {
  try {
    currencies.value = (await ledgerClient.listCurrencies({})).currencies
  } catch (err) {
    failure(err, 'Could not load currencies')
  }
}

async function saveCurrency(): Promise<void> {
  const form = currencyForm.value
  try {
    await ledgerClient.saveCurrency({
      currency: { code: form.code, label: form.label, exponent: form.exponent, disabled: form.disabled },
      expectedRevision: form.revision,
    })
    toast.success(`Saved currency ${form.code}.`)
    currencyForm.value = { code: '', label: '', exponent: 2, disabled: false, revision: BigInt(0) }
    currencyOpen.value = false
    await loadCurrencies()
  } catch (err) {
    failure(err, 'Could not save the currency')
  }
}

function editCurrency(currency: Currency): void {
  currencyForm.value = { code: currency.code, label: currency.label, exponent: currency.exponent, disabled: currency.disabled, revision: currency.revision }
}

const currencySearch = ref('')
const currencyFilteredActive = computed(() => currencySearch.value.trim() !== '')
const filteredCurrencies = computed(() => {
  const needle = currencySearch.value.trim().toLowerCase()
  return currencies.value.filter((currency) => !needle || `${currency.code} ${currency.label}`.toLowerCase().includes(needle))
})
const currencyColumns = [
  {
    id: 'code', header: 'Code', accessorFn: (currency: Currency) => currency.code.toLowerCase(), meta: { label: 'Code' },
    cell: ({ row }: { row: { original: Currency } }) => primaryCode(row.original.code, row.original.label),
  },
  {
    id: 'exponent', header: 'Decimal places', accessorFn: (currency: Currency) => currency.exponent, meta: { label: 'Decimal places', numeric: true },
    cell: ({ row }: { row: { original: Currency } }) => String(row.original.exponent),
  },
]

// The §16 currency sheet: rows open it to view and edit, New currency
// opens it blank. Closing with edits asks first.
const currencyOpen = ref(false)
const currencyCreating = ref(false)
const currencySnapshot = ref('')
const discardingCurrency = ref(false)

function snapshotCurrency(): string {
  const form = currencyForm.value
  return JSON.stringify({ code: form.code, label: form.label, exponent: form.exponent, disabled: form.disabled })
}
const currencyDirty = computed(() => currencyOpen.value && snapshotCurrency() !== currencySnapshot.value)

function openCurrency(currency?: Currency): void {
  if (currency) {
    editCurrency(currency)
    currencyCreating.value = false
  } else {
    currencyForm.value = { code: '', label: '', exponent: 2, disabled: false, revision: BigInt(0) }
    currencyCreating.value = true
  }
  currencySnapshot.value = snapshotCurrency()
  currencyOpen.value = true
}

function requestCurrencyClose(): void {
  if (currencyDirty.value) discardingCurrency.value = true
  else currencyOpen.value = false
}

async function saveRate(): Promise<void> {
  const form = rateForm.value
  try {
    await ledgerClient.saveRate({ rate: { fromCode: form.from, toCode: form.to, rateType: form.rateType, source: form.source, effectiveAt: form.at, rate: form.rate } })
    toast.success(`Saved ${form.from}->${form.to}.`)
    rateForm.value = { from: '', to: '', rateType: 'spot', source: '', at: '', rate: '' }
  } catch (err) {
    failure(err, 'Could not save the rate')
  }
}

async function lookupRate(): Promise<void> {
  const form = rateForm.value
  try {
    lookedUpRate.value = (await ledgerClient.getRate({ fromCode: form.from, toCode: form.to, rateType: form.rateType, source: form.source, at: form.at })).rate
  } catch (err) {
    lookedUpRate.value = ''
    failure(err, 'Could not resolve the rate')
  }
}

// Accounts, types and determination mappings.
const accountTypes = ref<AccountType[]>([])
const accounts = ref<Account[]>([])
const determinations = ref<Determination[]>([])
const accountTypeForm = ref({ name: '', label: '', normalBalance: 'debit' })
const accountForm = ref({ name: '', label: '', type: '', parent: '', isGroup: false, class: '', controlRole: '', company: '', disabled: false, revision: BigInt(0) })
const determinationForm = ref({ id: '', label: '', account: '', match: '', disabled: false, revision: BigInt(0) })
const resolveContext = ref('')
const resolvedAccount = ref('')

async function loadAccounts(): Promise<void> {
  const company = currentCompany.value
  try {
    const [types, list, mappings] = await Promise.all([
      ledgerClient.listAccountTypes({}),
      ledgerClient.listAccounts({ company }),
      ledgerClient.listDeterminations({}),
    ])
    if (currentCompany.value !== company) return
    accountTypes.value = types.accountTypes
    accounts.value = list.accounts
    determinations.value = mappings.determinations
  } catch (err) {
    if (currentCompany.value === company) failure(err, 'Could not load accounts')
  }
}

async function saveAccountType(): Promise<void> {
  const form = accountTypeForm.value
  try {
    await ledgerClient.saveAccountType({ accountType: { name: form.name, label: form.label, normalBalance: form.normalBalance } })
    toast.success(`Saved account type ${form.name}.`)
    accountTypeForm.value = { name: '', label: '', normalBalance: 'debit' }
    await loadAccounts()
  } catch (err) {
    failure(err, 'Could not save the account type')
  }
}

async function saveAccount(): Promise<void> {
  const form = accountForm.value
  try {
    await ledgerClient.saveAccount({
      account: { name: form.name, label: form.label, type: form.type, parent: form.parent, isGroup: form.isGroup, class: form.class, controlRole: form.controlRole, company: form.company || currentCompany.value, disabled: form.disabled },
      expectedRevision: form.revision,
    })
    toast.success(`Saved account ${form.name}.`)
    accountForm.value = { name: '', label: '', type: '', parent: '', isGroup: false, class: '', controlRole: '', company: '', disabled: false, revision: BigInt(0) }
    accountOpen.value = false
    await loadAccounts()
  } catch (err) {
    failure(err, 'Could not save the account')
  }
}

function editAccount(account: Account): void {
  accountForm.value = { name: account.name, label: account.label, type: account.type, parent: account.parent, isGroup: account.isGroup, class: account.class, controlRole: account.controlRole, company: account.company, disabled: account.disabled, revision: account.revision }
}

// The §16 account sheet: rows open it to view and edit, New account opens
// it blank. Closing with edits asks first.
const accountOpen = ref(false)
const accountCreating = ref(false)
const accountSnapshot = ref('')
const discardingAccount = ref(false)

function snapshotAccount(): string {
  const form = accountForm.value
  return JSON.stringify({ name: form.name, label: form.label, type: form.type, parent: form.parent, isGroup: form.isGroup, class: form.class, controlRole: form.controlRole, company: form.company })
}
const accountDirty = computed(() => accountOpen.value && snapshotAccount() !== accountSnapshot.value)

function openAccount(account?: Account): void {
  if (account) {
    editAccount(account)
    accountCreating.value = false
  } else {
    accountForm.value = { name: '', label: '', type: '', parent: '', isGroup: false, class: '', controlRole: '', company: '', disabled: false, revision: BigInt(0) }
    accountCreating.value = true
  }
  accountSnapshot.value = snapshotAccount()
  accountOpen.value = true
}

function requestAccountClose(): void {
  if (accountDirty.value) discardingAccount.value = true
  else accountOpen.value = false
}

function parsePairs(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const pair of text.split(',')) {
    const trimmed = pair.trim()
    if (!trimmed) continue
    const eq = trimmed.indexOf('=')
    if (eq < 0) throw new Error(`${trimmed} must be k=v`)
    out[trimmed.slice(0, eq).trim()] = trimmed.slice(eq + 1).trim()
  }
  return out
}

async function saveDetermination(): Promise<void> {
  const form = determinationForm.value
  try {
    await ledgerClient.saveDetermination({
      determination: { id: form.id, label: form.label, account: form.account, match: parsePairs(form.match), disabled: form.disabled },
      expectedRevision: form.revision,
    })
    toast.success(`Saved posting rule ${form.label}.`)
    determinationForm.value = { id: '', label: '', account: '', match: '', disabled: false, revision: BigInt(0) }
    await loadAccounts()
  } catch (err) {
    failure(err, 'Could not save the determination')
  }
}

function editDetermination(determination: Determination): void {
  determinationForm.value = { id: determination.id, label: determination.label, account: determination.account, match: Object.entries(determination.match).map(([k, v]) => `${k}=${v}`).join(','), disabled: determination.disabled, revision: determination.revision }
}

const accountTypeSearch = ref('')
const accountTypeFilteredActive = computed(() => accountTypeSearch.value.trim() !== '')
const filteredAccountTypes = computed(() => {
  const needle = accountTypeSearch.value.trim().toLowerCase()
  return accountTypes.value.filter((type) => !needle || `${type.name} ${type.label}`.toLowerCase().includes(needle))
})
const accountTypeColumns = [
  {
    id: 'code', header: 'Code', accessorFn: (type: AccountType) => type.name.toLowerCase(), meta: { label: 'Code' },
    cell: ({ row }: { row: { original: AccountType } }) => primaryCode(row.original.name, row.original.label),
  },
  {
    id: 'balance', header: 'Usual balance', accessorFn: (type: AccountType) => type.normalBalance, meta: { label: 'Usual balance' },
    cell: ({ row }: { row: { original: AccountType } }) => row.original.normalBalance,
  },
]

const accountSearch = ref('')
const accountFilteredActive = computed(() => accountSearch.value.trim() !== '')
const filteredAccounts = computed(() => {
  const needle = accountSearch.value.trim().toLowerCase()
  return accounts.value.filter((account) => !needle || `${account.name} ${account.label} ${account.type}`.toLowerCase().includes(needle))
})
const accountColumns = [
  {
    id: 'code', header: 'Code', accessorFn: (account: Account) => account.name.toLowerCase(), meta: { label: 'Code' },
    cell: ({ row }: { row: { original: Account } }) => primaryCode(row.original.name, row.original.label),
  },
  {
    id: 'type', header: 'Type', accessorFn: (account: Account) => account.type, meta: { label: 'Type' },
    cell: ({ row }: { row: { original: Account } }) => row.original.type,
  },
  {
    id: 'parent', header: 'Belongs under', accessorFn: (account: Account) => account.parent, meta: { label: 'Belongs under' },
    cell: ({ row }: { row: { original: Account } }) => dashCell(row.original.parent),
  },
  {
    id: 'group', header: 'Group', accessorFn: (account: Account) => (account.isGroup ? 'yes' : 'no'), meta: { label: 'Group' },
    cell: ({ row }: { row: { original: Account } }) => (row.original.isGroup ? 'yes' : 'no'),
  },
]

const determinationSearch = ref('')
const determinationFilteredActive = computed(() => determinationSearch.value.trim() !== '')
const filteredDeterminations = computed(() => {
  const needle = determinationSearch.value.trim().toLowerCase()
  return determinations.value.filter((determination) => !needle || `${determination.label} ${determination.account}`.toLowerCase().includes(needle))
})
const determinationColumns = [
  {
    id: 'name', header: 'Name', accessorFn: (determination: Determination) => determination.label.toLowerCase(), meta: { label: 'Name' },
    cell: ({ row }: { row: { original: Determination } }) => h(PrimaryCell, { title: row.original.label }),
  },
  {
    id: 'account', header: 'Account', accessorFn: (determination: Determination) => determination.account, meta: { label: 'Account' },
    cell: ({ row }: { row: { original: Determination } }) => h('code', null, row.original.account),
  },
  {
    id: 'match', header: 'Matches when', accessorFn: (determination: Determination) => Object.entries(determination.match).map(([k, v]) => `${k}=${v}`).join(', '), meta: { label: 'Matches when' },
    cell: ({ row }: { row: { original: Determination } }) => Object.entries(row.original.match).map(([k, v]) => `${k}=${v}`).join(', ') || 'always',
  },
]

async function resolvePreview(): Promise<void> {
  try {
    resolvedAccount.value = (await ledgerClient.resolveAccount({ context: parsePairs(resolveContext.value) })).determination?.account ?? ''
    if (!resolvedAccount.value) toast.error('Nothing matches that context.')
  } catch (err) {
    resolvedAccount.value = ''
    failure(err, 'Could not resolve an account')
  }
}

// Readiness of the current company.
const readiness = ref<ReadinessCheck[]>([])

async function loadReadiness(): Promise<void> {
  const company = currentCompany.value
  if (!company) {
    readiness.value = []
    return
  }
  try {
    const res = await ledgerClient.getReadiness({ company })
    if (currentCompany.value === company) readiness.value = res.checks
  } catch (err) {
    if (currentCompany.value === company) failure(err, 'Could not load readiness')
  }
}

const hasCompany = computed(() => currentCompany.value !== '')
const ledgerTab = ref('readiness')
const initialLoaded = ref(false)

function onLedgerTab(tab: unknown): void {
  if (typeof tab !== 'string') return
  ledgerTab.value = tab
  if (tab === 'readiness') void loadReadiness()
}

watch(currentCompany, () => {
  void loadOrg()
  void loadPeriods()
  void loadAccounts()
  void loadReadiness()
})

onMounted(async () => {
  try {
    await loadCompanies()
    await Promise.all([loadOrg(), loadDimensions(), loadPeriods(), loadCurrencies(), loadAccounts(), loadReadiness()])
  } finally {
    initialLoaded.value = true
  }
})
</script>

<template>
  <div class="page-content">
    <div class="page-title-row"><div><h1>Ledger</h1><p class="lead">Set up your companies, periods, currencies and accounts before you record money.</p></div></div>
    <Tabs :model-value="ledgerTab" @update:model-value="onLedgerTab">
      <TabsList>
        <TabsTrigger value="readiness">Readiness</TabsTrigger>
        <TabsTrigger value="companies">Companies</TabsTrigger>
        <TabsTrigger value="periods">Periods</TabsTrigger>
        <TabsTrigger value="currencies">Currencies</TabsTrigger>
        <TabsTrigger value="accounts">Accounts</TabsTrigger>
      </TabsList>
      <TabsContent value="readiness">
        <div class="ledger-stack">
          <Card>
            <CardHeader><CardTitle>Ready to post? <HelpHint term="posting" /></CardTitle><CardDescription>Everything a company needs before it can keep books. Fix the missing rows first.</CardDescription></CardHeader>
            <CardContent>
              <div class="settings-grid">
                <SelectField id="ledger-readiness-company" label="Company" v-model="currentCompany" placeholder="Choose a company" :options="companies.map((company) => ({ value: company.name, label: company.label }))" />
              </div>
              <Table v-if="readiness.length > 0" class="datatable-stacked">
                <TableHeader><TableRow><TableHead>Check</TableHead><TableHead>Status</TableHead><TableHead>Next step</TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="check in readiness" :key="check.name"><TableCell data-label="Check">{{ check.name }}</TableCell><TableCell data-label="Status"><Badge :variant="check.ok ? 'secondary' : 'outline'">{{ check.ok ? 'ready' : 'missing' }}</Badge></TableCell><TableCell data-label="Next step"><code v-if="!check.ok">{{ check.detail }}</code><span v-else class="muted-label">{{ check.detail }}</span></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">Choose a company to see its readiness.</p>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
      <TabsContent value="companies">
        <div class="ledger-stack">
          <Card>
            <CardHeader><CardTitle>Companies</CardTitle><CardDescription>Each company keeps its own books. Click a row to view and edit it.</CardDescription></CardHeader>
            <CardContent>
              <DataTable
                :columns="companyColumns"
                :data="filteredCompanies"
                table-id="ledger-companies"
                :username="username"
                :get-row-id="(company) => company.name"
                v-model:search="companySearch"
                search-placeholder="Find companies…"
                :loading="!initialLoaded"
                :filtered="companyFilteredActive"
                clickable
                empty-title="No companies yet"
                empty-body="Create one to get started."
                empty-art="empty-ledger"
                filtered-art="empty-sieve"
                @row-click="openCompany"
              >
                <template v-if="props.canWrite" #toolbar-actions>
                  <ActionButton kind="primary" density="row" label="New company" icon="plus" @press="openCompany()" />
                </template>
                <template v-if="props.canWrite" #empty-action>
                  <ActionButton kind="primary" label="Create your first company" icon="plus" @press="openCompany()" />
                </template>
              </DataTable>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Branches</CardTitle><CardDescription>Places or teams inside {{ currentCompany || 'the company' }}.</CardDescription></CardHeader>
            <CardContent>
              <DataTable
                :columns="branchColumns"
                :data="filteredBranches"
                table-id="ledger-branches"
                :username="username"
                :get-row-id="(branch) => branch.name"
                v-model:search="branchSearch"
                search-placeholder="Find branches…"
                :loading="!initialLoaded"
                :filtered="branchFilteredActive"
                clickable
                empty-title="No branches yet"
                empty-body="Add one with the form below."
                @row-click="editBranch"
              />
              <div v-if="props.canWrite && hasCompany" class="settings-grid">
                <TextField id="ledger-branch-name" label="Code" v-model="branchForm.name" />
                <TextField id="ledger-branch-label" label="Name" v-model="branchForm.label" />
                <TextField id="ledger-branch-parent" label="Belongs under" optional v-model="branchForm.parent" help="A group branch above this one, if any." />
                <CheckboxField id="ledger-branch-group" label="This is a group" v-model="branchForm.isGroup" help="Groups organize. Only the rest can take postings." />
              </div>
              <div v-if="props.canWrite && hasCompany" class="ledger-actions"><ActionButton kind="primary" density="row" label="Save branch" icon="save" @press="saveBranch" /></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Departments</CardTitle><CardDescription>Who spends and earns inside {{ currentCompany || 'the company' }}.</CardDescription></CardHeader>
            <CardContent>
              <DataTable
                :columns="departmentColumns"
                :data="filteredDepartments"
                table-id="ledger-departments"
                :username="username"
                :get-row-id="(department) => department.name"
                v-model:search="departmentSearch"
                search-placeholder="Find departments…"
                :loading="!initialLoaded"
                :filtered="departmentFilteredActive"
                clickable
                empty-title="No departments yet"
                empty-body="Add one with the form below."
                @row-click="editDepartment"
              />
              <div v-if="props.canWrite && hasCompany" class="settings-grid">
                <TextField id="ledger-department-name" label="Code" v-model="departmentForm.name" />
                <TextField id="ledger-department-label" label="Name" v-model="departmentForm.label" />
                <TextField id="ledger-department-branch" label="Branch" optional v-model="departmentForm.branch" help="Which branch this sits in, if any." />
                <TextField id="ledger-department-parent" label="Belongs under" optional v-model="departmentForm.parent" />
                <CheckboxField id="ledger-department-group" label="This is a group" v-model="departmentForm.isGroup" help="Groups organize. Only the rest can take postings." />
              </div>
              <div v-if="props.canWrite && hasCompany" class="ledger-actions"><ActionButton kind="primary" density="row" label="Save department" icon="save" @press="saveDepartment" /></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Cost centres</CardTitle><CardDescription>What costs are tracked against.</CardDescription></CardHeader>
            <CardContent>
              <DataTable
                :columns="costCentreColumns"
                :data="filteredCostCentres"
                table-id="ledger-cost-centres"
                :username="username"
                :get-row-id="(centre) => centre.name"
                v-model:search="costCentreSearch"
                search-placeholder="Find cost centres…"
                :loading="!initialLoaded"
                :filtered="costCentreFilteredActive"
                clickable
                empty-title="No cost centres yet"
                empty-body="Add one with the form below."
                @row-click="editCostCentre"
              />
              <div v-if="props.canWrite && hasCompany" class="settings-grid">
                <TextField id="ledger-cc-name" label="Code" v-model="costCentreForm.name" />
                <TextField id="ledger-cc-label" label="Name" v-model="costCentreForm.label" />
                <TextField id="ledger-cc-parent" label="Belongs under" optional v-model="costCentreForm.parent" />
                <CheckboxField id="ledger-cc-group" label="This is a group" v-model="costCentreForm.isGroup" help="Groups organize. Only the rest can take postings." />
              </div>
              <div v-if="props.canWrite && hasCompany" class="ledger-actions"><ActionButton kind="primary" density="row" label="Save cost centre" icon="save" @press="saveCostCentre" /></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Dimensions</CardTitle><CardDescription>Extra labels a posting can carry, like project or region.</CardDescription></CardHeader>
            <CardContent>
              <DataTable
                :columns="dimensionColumns"
                :data="filteredDimensions"
                table-id="ledger-dimensions"
                :username="username"
                :get-row-id="(dimension) => dimension.name"
                v-model:search="dimensionSearch"
                search-placeholder="Find dimensions…"
                :loading="!initialLoaded"
                :filtered="dimensionFilteredActive"
                clickable
                empty-title="No dimensions yet"
                empty-body="Add one with the form below."
                @row-click="editDimension"
              />
              <div v-if="props.canWrite" class="settings-grid">
                <TextField id="ledger-dimension-name" label="Code" v-model="dimensionForm.name" />
                <TextField id="ledger-dimension-label" label="Name" v-model="dimensionForm.label" />
                <TextField id="ledger-dimension-members" label="Allowed values" v-model="dimensionForm.members" help="Comma separated, e.g. website,retail." />
                <TextField id="ledger-dimension-mandatory" label="Always needed for" v-model="dimensionForm.mandatoryFor" help="Accounts whose postings must pick a value, comma separated." />
                <DateField id="ledger-dimension-from" label="Valid from" optional v-model="dimensionForm.validFrom" placeholder="YYYY-MM-DD" help="Leave empty for always." />
                <DateField id="ledger-dimension-to" label="Valid to" optional v-model="dimensionForm.validTo" placeholder="YYYY-MM-DD" />
              </div>
              <div v-if="props.canWrite"><ActionButton kind="primary" density="row" label="Save dimension" icon="save" @press="saveDimension" /></div>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
      <TabsContent value="periods">
        <div class="ledger-stack">
          <Card>
            <CardHeader><CardTitle>Financial years</CardTitle><CardDescription>The year each company's calendar follows.</CardDescription></CardHeader>
            <CardContent>
              <div class="settings-grid">
                <TextField id="ledger-book" label="Book" v-model="book" placeholder="main" help="Which set of books. Usually main." @blur="loadPeriods" />
              </div>
              <DataTable
                :columns="fiscalYearColumns"
                :data="filteredFiscalYears"
                table-id="ledger-fiscal-years"
                :username="username"
                :get-row-id="(year) => year.name"
                v-model:search="fiscalYearSearch"
                search-placeholder="Find financial years…"
                :loading="!initialLoaded"
                :filtered="fiscalYearFilteredActive"
                clickable
                empty-title="No financial years yet"
                empty-body="Add one with the form below."
                empty-art="empty-ledger"
                filtered-art="empty-sieve"
                @row-click="editFiscalYear"
              />
              <div v-if="props.canWrite && hasCompany" class="settings-grid">
                <TextField id="ledger-fy-name" label="Code" v-model="fiscalYearForm.name" />
                <TextField id="ledger-fy-label" label="Name" v-model="fiscalYearForm.label" />
                <DateField id="ledger-fy-start" label="First day" v-model="fiscalYearForm.startDate" placeholder="YYYY-MM-DD" />
                <DateField id="ledger-fy-end" label="Last day" v-model="fiscalYearForm.endDate" placeholder="YYYY-MM-DD" />
              </div>
              <div v-if="props.canWrite && hasCompany" class="ledger-actions"><ActionButton kind="primary" density="row" label="Save financial year" icon="save" @press="saveFiscalYear" /></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Periods</CardTitle><CardDescription>Posting windows. Dates never overlap within one book. Close a period to stop new postings; a hard close cannot be undone here.</CardDescription></CardHeader>
            <CardContent>
              <DataTable
                :columns="periodColumns"
                :data="filteredPeriods"
                table-id="ledger-periods"
                :username="username"
                :get-row-id="(period) => period.name"
                v-model:search="periodSearch"
                search-placeholder="Find periods…"
                :filters="periodFilters"
                :loading="!initialLoaded"
                :filtered="periodFilteredActive"
                clickable
                empty-title="No periods yet"
                empty-body="Create one to get started."
                @row-click="openPeriod"
                @update:filters="onPeriodFiltersChanged"
              >
                <template v-if="props.canWrite && hasCompany" #toolbar-actions>
                  <ActionButton kind="primary" density="row" label="New period" icon="plus" @press="openPeriod()" />
                </template>
                <template v-if="props.canWrite && hasCompany" #empty-action>
                  <ActionButton kind="primary" label="Create your first period" icon="plus" @press="openPeriod()" />
                </template>
              </DataTable>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
      <TabsContent value="currencies">
        <div class="ledger-stack">
          <Card>
            <CardHeader><CardTitle>Currencies</CardTitle><CardDescription>Money the books can use.</CardDescription></CardHeader>
            <CardContent>
              <DataTable
                :columns="currencyColumns"
                :data="filteredCurrencies"
                table-id="ledger-currencies"
                :username="username"
                :get-row-id="(currency) => currency.code"
                v-model:search="currencySearch"
                search-placeholder="Find currencies…"
                :loading="!initialLoaded"
                :filtered="currencyFilteredActive"
                clickable
                empty-title="No currencies yet"
                empty-body="Create one to get started."
                empty-art="empty-ledger"
                filtered-art="empty-sieve"
                @row-click="openCurrency"
              >
                <template v-if="props.canWrite" #toolbar-actions>
                  <ActionButton kind="primary" density="row" label="New currency" icon="plus" @press="openCurrency()" />
                </template>
                <template v-if="props.canWrite" #empty-action>
                  <ActionButton kind="primary" label="Create your first currency" icon="plus" @press="openCurrency()" />
                </template>
              </DataTable>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Exchange rates</CardTitle><CardDescription>What one money is worth in another, from a moment on. Rates are never edited; save a new one instead.</CardDescription></CardHeader>
            <CardContent>
              <div class="settings-grid">
                <TextField id="ledger-rate-from" label="From" v-model="rateForm.from" placeholder="USD" />
                <TextField id="ledger-rate-to" label="To" v-model="rateForm.to" placeholder="EUR" />
                <TextField id="ledger-rate-type" label="Kind of rate" v-model="rateForm.rateType" help="spot, average…" />
                <TextField id="ledger-rate-source" label="Source" v-model="rateForm.source" help="Where the rate came from." />
                <TextField id="ledger-rate-at" label="Starting" v-model="rateForm.at" placeholder="2026-01-01T00:00:00Z" help="Date and time, e.g. 2026-01-01T00:00:00Z." />
                <NumberField id="ledger-rate-rate" label="Rate" v-model="rateForm.rate" />
              </div>
              <div class="ledger-actions"><ActionButton v-if="props.canWrite" kind="primary" density="row" label="Save rate" icon="save" @press="saveRate" /> <ActionButton kind="secondary" density="row" label="Look up" icon="search" @press="lookupRate" /> <code v-if="lookedUpRate">{{ lookedUpRate }}</code></div>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
      <TabsContent value="accounts">
        <div class="ledger-stack">
          <Card>
            <CardHeader><CardTitle>Account types</CardTitle><CardDescription>Kinds of account, like asset or income.</CardDescription></CardHeader>
            <CardContent>
              <DataTable
                :columns="accountTypeColumns"
                :data="filteredAccountTypes"
                table-id="ledger-account-types"
                :username="username"
                :get-row-id="(type) => type.name"
                v-model:search="accountTypeSearch"
                search-placeholder="Find account types…"
                :loading="!initialLoaded"
                :filtered="accountTypeFilteredActive"
                empty-title="No account types yet"
                empty-body="Add one with the form below."
                empty-art="empty-ledger"
                filtered-art="empty-sieve"
              />
              <div v-if="props.canWrite" class="settings-grid">
                <TextField id="ledger-type-name" label="Code" v-model="accountTypeForm.name" />
                <TextField id="ledger-type-label" label="Name" v-model="accountTypeForm.label" />
                <TextField id="ledger-type-balance" label="Usual balance" v-model="accountTypeForm.normalBalance" placeholder="debit" help="Which side balances usually sit on: debit or credit." />
              </div>
              <div v-if="props.canWrite" class="ledger-actions"><ActionButton kind="primary" density="row" label="Save account type" icon="save" @press="saveAccountType" /></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Accounts</CardTitle><CardDescription>The chart of accounts. Groups organize; only the rest can take postings.</CardDescription></CardHeader>
            <CardContent>
              <DataTable
                :columns="accountColumns"
                :data="filteredAccounts"
                table-id="ledger-accounts"
                :username="username"
                :get-row-id="(account) => account.name"
                v-model:search="accountSearch"
                search-placeholder="Find accounts…"
                :loading="!initialLoaded"
                :filtered="accountFilteredActive"
                clickable
                empty-title="No accounts yet"
                empty-body="Create one to get started."
                @row-click="openAccount"
              >
                <template v-if="props.canWrite" #toolbar-actions>
                  <ActionButton kind="primary" density="row" label="New account" icon="plus" @press="openAccount()" />
                </template>
                <template v-if="props.canWrite" #empty-action>
                  <ActionButton kind="primary" label="Create your first account" icon="plus" @press="openAccount()" />
                </template>
              </DataTable>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Posting rules</CardTitle><CardDescription>Decide which account a posting uses. More detail wins; ties are refused.</CardDescription></CardHeader>
            <CardContent>
              <DataTable
                :columns="determinationColumns"
                :data="filteredDeterminations"
                table-id="ledger-determinations"
                :username="username"
                :get-row-id="(determination) => determination.id"
                v-model:search="determinationSearch"
                search-placeholder="Find posting rules…"
                :loading="!initialLoaded"
                :filtered="determinationFilteredActive"
                clickable
                empty-title="No posting rules yet"
                empty-body="Add one with the form below."
                @row-click="editDetermination"
              />
              <div v-if="props.canWrite" class="settings-grid">
                <TextField id="ledger-det-label" label="Name" v-model="determinationForm.label" help="A name you'll recognize." />
                <TextField id="ledger-det-account" label="Account" v-model="determinationForm.account" />
                <TextField id="ledger-det-match" label="Matches when" optional v-model="determinationForm.match" help="Posting details, e.g. company=acme. Empty matches everything." />
              </div>
              <div v-if="props.canWrite"><ActionButton kind="primary" density="row" label="Save posting rule" icon="save" @press="saveDetermination" /></div>
              <div class="settings-grid">
                <TextField id="ledger-resolve-context" label="Posting details" v-model="resolveContext" help="Try a rule: details like company=acme, then Check." />
              </div>
              <div class="ledger-actions"><ActionButton kind="secondary" density="row" label="Check" icon="circle-check" @press="resolvePreview" /> <code v-if="resolvedAccount">Uses account: {{ resolvedAccount }}</code></div>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
    </Tabs>
    <ConfirmDialog
      :open="hardClosing !== null"
      :title="`Hard close ${hardClosing?.label || hardClosing?.name}?`"
      body="Nobody can record or change anything in this period again, and it cannot be reopened."
      confirm-label="Hard close"
      cancel-label="Keep it open"
      confirm-icon="lock"
      @confirm="confirmHardClose"
      @update:open="(value: boolean) => { if (!value) hardClosing = null }"
    />

    <DetailSheet
      v-if="companyOpen"
      :open="true"
      :title="companyCreating ? 'New company' : (companyForm.label || companyForm.name)"
      @update:open="(value: boolean) => { if (!value) requestCompanyClose() }"
    >
      <template v-if="!companyCreating" #subtitle>
        <p class="text-caption text-muted-foreground font-mono">{{ companyForm.name }}</p>
      </template>
      <template v-if="companyForm.disabled && !companyCreating" #status>
        <Badge variant="outline">Inactive</Badge>
      </template>
      <div class="sheet-form">
        <TextField id="ledger-company-name" label="Code" v-model="companyForm.name" placeholder="acme" help="Short unique code. Never changes." :disabled="!props.canWrite" />
        <TextField id="ledger-company-label" label="Name" v-model="companyForm.label" placeholder="Acme" :disabled="!props.canWrite" />
        <TextField id="ledger-company-base" label="Main money" v-model="companyForm.baseCurrency" placeholder="USD" help="The money this company reports in." :disabled="!props.canWrite" />
        <CheckboxField id="ledger-company-disabled" label="Inactive" v-model="companyForm.disabled" help="Inactive items stay for history but can't be used." :disabled="!props.canWrite" />
      </div>
      <template #footer>
        <ActionButton kind="secondary" label="Cancel" icon="x" @press="companyOpen = false" />
        <ActionButton v-if="props.canWrite" kind="primary" label="Save company" icon="save" @press="saveCompany" />
      </template>
    </DetailSheet>
    <ConfirmDialog
      :open="discardingCompany"
      title="Discard your edits?"
      body="The company has changes that are not saved."
      confirm-label="Discard edits"
      cancel-label="Keep editing"
      confirm-icon="x"
      @confirm="discardingCompany = false; companyOpen = false"
      @update:open="(value: boolean) => { if (!value) discardingCompany = false }"
    />

    <DetailSheet
      v-if="periodOpen"
      :open="true"
      :title="periodCreating ? 'New period' : (periodForm.label || periodForm.name)"
      @update:open="(value: boolean) => { if (!value) requestPeriodClose() }"
    >
      <template v-if="!periodCreating" #subtitle>
        <p class="text-caption text-muted-foreground font-mono">{{ periodForm.name }}</p>
      </template>
      <template v-if="!periodCreating && periodTarget" #status>
        <Badge :variant="periodTarget.status === 'open' ? 'secondary' : 'outline'">{{ periodTarget.status }}</Badge>
      </template>
      <div class="sheet-form">
        <TextField id="ledger-period-name" label="Code" v-model="periodForm.name" :disabled="!props.canWrite" />
        <TextField id="ledger-period-label" label="Name" v-model="periodForm.label" :disabled="!props.canWrite" />
        <TextField id="ledger-period-fy" label="Financial year" v-model="periodForm.fiscalYear" :disabled="!props.canWrite" />
        <DateField id="ledger-period-start" label="First day" v-model="periodForm.startDate" placeholder="YYYY-MM-DD" :disabled="!props.canWrite" />
        <DateField id="ledger-period-end" label="Last day" v-model="periodForm.endDate" placeholder="YYYY-MM-DD" :disabled="!props.canWrite" />
      </div>
      <template #footer>
        <ActionGroup :actions="periodActions" stack-on-phones aria-label="Period actions" @press="onPeriodAction" />
      </template>
    </DetailSheet>
    <ConfirmDialog
      :open="discardingPeriod"
      title="Discard your edits?"
      body="The period has changes that are not saved."
      confirm-label="Discard edits"
      cancel-label="Keep editing"
      confirm-icon="x"
      @confirm="discardingPeriod = false; periodOpen = false"
      @update:open="(value: boolean) => { if (!value) discardingPeriod = false }"
    />

    <DetailSheet
      v-if="currencyOpen"
      :open="true"
      :title="currencyCreating ? 'New currency' : (currencyForm.label || currencyForm.code)"
      @update:open="(value: boolean) => { if (!value) requestCurrencyClose() }"
    >
      <template v-if="!currencyCreating" #subtitle>
        <p class="text-caption text-muted-foreground font-mono">{{ currencyForm.code }}</p>
      </template>
      <template v-if="currencyForm.disabled && !currencyCreating" #status>
        <Badge variant="outline">Inactive</Badge>
      </template>
      <div class="sheet-form">
        <TextField id="ledger-currency-code" label="Code" v-model="currencyForm.code" placeholder="USD" help="Three letters, e.g. USD." :disabled="!props.canWrite" />
        <TextField id="ledger-currency-label" label="Name" v-model="currencyForm.label" :disabled="!props.canWrite" />
        <NumberField id="ledger-currency-exponent" label="Decimal places" :model-value="String(currencyForm.exponent)" help="Digits after the point: 2 for most money." :disabled="!props.canWrite" @update:model-value="currencyForm.exponent = Number($event)" />
        <CheckboxField id="ledger-currency-disabled" label="Inactive" v-model="currencyForm.disabled" :disabled="!props.canWrite" />
      </div>
      <template #footer>
        <ActionButton kind="secondary" label="Cancel" icon="x" @press="currencyOpen = false" />
        <ActionButton v-if="props.canWrite" kind="primary" label="Save currency" icon="save" @press="saveCurrency" />
      </template>
    </DetailSheet>
    <ConfirmDialog
      :open="discardingCurrency"
      title="Discard your edits?"
      body="The currency has changes that are not saved."
      confirm-label="Discard edits"
      cancel-label="Keep editing"
      confirm-icon="x"
      @confirm="discardingCurrency = false; currencyOpen = false"
      @update:open="(value: boolean) => { if (!value) discardingCurrency = false }"
    />

    <DetailSheet
      v-if="accountOpen"
      :open="true"
      :title="accountCreating ? 'New account' : (accountForm.label || accountForm.name)"
      @update:open="(value: boolean) => { if (!value) requestAccountClose() }"
    >
      <template v-if="!accountCreating" #subtitle>
        <p class="text-caption text-muted-foreground font-mono">{{ accountForm.name }}</p>
      </template>
      <template v-if="!accountCreating" #status>
        <Badge variant="secondary">{{ accountForm.type || 'No type' }}</Badge>
      </template>
      <div class="sheet-form">
        <TextField id="ledger-account-name" label="Code" v-model="accountForm.name" :disabled="!props.canWrite" />
        <TextField id="ledger-account-label" label="Name" v-model="accountForm.label" :disabled="!props.canWrite" />
        <TextField id="ledger-account-type" label="Type" v-model="accountForm.type" :disabled="!props.canWrite" />
        <TextField id="ledger-account-parent" label="Belongs under" optional v-model="accountForm.parent" help="A group account above this one, if any." :disabled="!props.canWrite" />
        <CheckboxField id="ledger-account-group" label="This is a group" v-model="accountForm.isGroup" help="Groups organize. Only the rest can take postings." :disabled="!props.canWrite" />
        <TextField id="ledger-account-class" label="Class" optional v-model="accountForm.class" help="Optional grouping used by reports." :disabled="!props.canWrite" />
        <TextField id="ledger-account-role" label="Controls" optional v-model="accountForm.controlRole" help="What this account controls, if anything." :disabled="!props.canWrite" />
        <TextField id="ledger-account-company" label="Company" optional v-model="accountForm.company" help="Empty means every company." :disabled="!props.canWrite" />
      </div>
      <template #footer>
        <ActionButton kind="secondary" label="Cancel" icon="x" @press="accountOpen = false" />
        <ActionButton v-if="props.canWrite" kind="primary" label="Save account" icon="save" @press="saveAccount" />
      </template>
    </DetailSheet>
    <ConfirmDialog
      :open="discardingAccount"
      title="Discard your edits?"
      body="The account has changes that are not saved."
      confirm-label="Discard edits"
      cancel-label="Keep editing"
      confirm-icon="x"
      @confirm="discardingAccount = false; accountOpen = false"
      @update:open="(value: boolean) => { if (!value) discardingAccount = false }"
    />
  </div>
</template>
