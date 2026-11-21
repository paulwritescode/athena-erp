<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { ledgerClient } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
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

const props = defineProps<{ canWrite: boolean }>()

function failure(err: unknown, action: string): void {
  toast.error(err instanceof ConnectError ? `${action}: ${err.rawMessage}` : `${action}.`)
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

async function saveCompany(): Promise<void> {
  const form = companyForm.value
  try {
    const res = await ledgerClient.saveCompany({
      company: { name: form.name, label: form.label, baseCurrency: form.baseCurrency, disabled: form.disabled },
      expectedRevision: form.revision,
    })
    toast.success(`Saved company ${res.company?.name}.`)
    resetCompanyForm()
    await loadCompanies()
    if (res.company) currentCompany.value = res.company.name
  } catch (err) {
    failure(err, 'Could not save the company')
  }
}

// Branches, departments and cost centres of the current company.
const branches = ref<Branch[]>([])
const departments = ref<Department[]>([])
const costCentres = ref<CostCentre[]>([])
const branchForm = ref({ name: '', label: '', parent: '', isGroup: false, revision: BigInt(0) })
const departmentForm = ref({ name: '', label: '', branch: '', parent: '', isGroup: false, revision: BigInt(0) })
const costCentreForm = ref({ name: '', label: '', parent: '', isGroup: false, revision: BigInt(0) })

async function loadOrg(): Promise<void> {
  if (!currentCompany.value) {
    branches.value = []
    departments.value = []
    costCentres.value = []
    return
  }
  try {
    const [b, d, c] = await Promise.all([
      ledgerClient.listBranches({ company: currentCompany.value }),
      ledgerClient.listDepartments({ company: currentCompany.value }),
      ledgerClient.listCostCentres({ company: currentCompany.value }),
    ])
    branches.value = b.branches
    departments.value = d.departments
    costCentres.value = c.costCentres
  } catch (err) {
    failure(err, 'Could not load organisation units')
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

// Fiscal years and periods of the current company.
const fiscalYears = ref<FiscalYear[]>([])
const periods = ref<Period[]>([])
const book = ref('main')
const fiscalYearForm = ref({ name: '', label: '', startDate: '', endDate: '', revision: BigInt(0) })
const periodForm = ref({ name: '', label: '', fiscalYear: '', startDate: '', endDate: '', revision: BigInt(0) })

async function loadPeriods(): Promise<void> {
  if (!currentCompany.value) {
    fiscalYears.value = []
    periods.value = []
    return
  }
  try {
    const [years, list] = await Promise.all([
      ledgerClient.listFiscalYears({ company: currentCompany.value }),
      ledgerClient.listPeriods({ company: currentCompany.value, book: book.value || 'main' }),
    ])
    fiscalYears.value = years.fiscalYears
    periods.value = list.periods
  } catch (err) {
    failure(err, 'Could not load fiscal years and periods')
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

async function savePeriod(): Promise<void> {
  const form = periodForm.value
  try {
    await ledgerClient.savePeriod({
      period: { company: currentCompany.value, book: book.value || 'main', name: form.name, label: form.label, fiscalYear: form.fiscalYear, startDate: form.startDate, endDate: form.endDate },
      expectedRevision: form.revision,
    })
    toast.success(`Saved period ${form.name}.`)
    periodForm.value = { name: '', label: '', fiscalYear: '', startDate: '', endDate: '', revision: BigInt(0) }
    await loadPeriods()
  } catch (err) {
    failure(err, 'Could not save the period')
  }
}

function editPeriod(period: Period): void {
  periodForm.value = { name: period.name, label: period.label, fiscalYear: period.fiscalYear, startDate: period.startDate, endDate: period.endDate, revision: period.revision }
}

async function setPeriodStatus(period: Period, status: string): Promise<void> {
  try {
    await ledgerClient.setPeriodStatus({ company: period.company, book: period.book, name: period.name, status, expectedRevision: period.revision })
    toast.success(`Period ${period.name} is now ${status}.`)
    await loadPeriods()
  } catch (err) {
    failure(err, 'Could not change the period status')
  }
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
    await loadCurrencies()
  } catch (err) {
    failure(err, 'Could not save the currency')
  }
}

function editCurrency(currency: Currency): void {
  currencyForm.value = { code: currency.code, label: currency.label, exponent: currency.exponent, disabled: currency.disabled, revision: currency.revision }
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
  try {
    const [types, list, mappings] = await Promise.all([
      ledgerClient.listAccountTypes({}),
      ledgerClient.listAccounts({ company: currentCompany.value }),
      ledgerClient.listDeterminations({}),
    ])
    accountTypes.value = types.accountTypes
    accounts.value = list.accounts
    determinations.value = mappings.determinations
  } catch (err) {
    failure(err, 'Could not load accounts')
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
    await loadAccounts()
  } catch (err) {
    failure(err, 'Could not save the account')
  }
}

function editAccount(account: Account): void {
  accountForm.value = { name: account.name, label: account.label, type: account.type, parent: account.parent, isGroup: account.isGroup, class: account.class, controlRole: account.controlRole, company: account.company, disabled: account.disabled, revision: account.revision }
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
  if (!currentCompany.value) {
    readiness.value = []
    return
  }
  try {
    readiness.value = (await ledgerClient.getReadiness({ company: currentCompany.value })).checks
  } catch (err) {
    failure(err, 'Could not load readiness')
  }
}

const hasCompany = computed(() => currentCompany.value !== '')
const ledgerTab = ref('readiness')

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
  await loadCompanies()
  await Promise.all([loadOrg(), loadDimensions(), loadPeriods(), loadCurrencies(), loadAccounts(), loadReadiness()])
})
</script>

<template>
  <div class="page-content">
    <div class="page-title-row"><div><h1>Ledger</h1><p class="lead">Companies, periods, currencies and accounts. Jurisdiction, chart and tax stay open for the sponsor.</p></div></div>
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
            <CardHeader><CardTitle>Ready to post?</CardTitle><CardDescription>Everything a company needs before it can keep books. Fix the missing rows first.</CardDescription></CardHeader>
            <CardContent>
              <div class="settings-grid">
                <div class="settings-field"><Label for="ledger-readiness-company">Company</Label><select id="ledger-readiness-company" v-model="currentCompany" class="ledger-select"><option value="" disabled>Choose a company</option><option v-for="company in companies" :key="company.name" :value="company.name">{{ company.label }}</option></select></div>
              </div>
              <Table v-if="readiness.length > 0">
                <TableHeader><TableRow><TableHead>Check</TableHead><TableHead>Status</TableHead><TableHead>Next step</TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="check in readiness" :key="check.name"><TableCell>{{ check.name }}</TableCell><TableCell><Badge :variant="check.ok ? 'secondary' : 'outline'">{{ check.ok ? 'ready' : 'missing' }}</Badge></TableCell><TableCell><code v-if="!check.ok">{{ check.detail }}</code><span v-else class="muted-label">{{ check.detail }}</span></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">Choose a company to see its readiness.</p>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
      <TabsContent value="companies">
        <div class="ledger-stack">
          <Card>
            <CardHeader><CardTitle>Companies</CardTitle><CardDescription>Each company keeps its own books. Click a row to work on it below.</CardDescription></CardHeader>
            <CardContent>
              <Table v-if="companies.length > 0">
                <TableHeader><TableRow><TableHead>Code</TableHead><TableHead>Name</TableHead><TableHead>Main money</TableHead><TableHead></TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="company in companies" :key="company.name" :class="{ chosen: company.name === currentCompany }"><TableCell><code>{{ company.name }}</code></TableCell><TableCell>{{ company.label }}</TableCell><TableCell>{{ company.baseCurrency || '-' }}</TableCell><TableCell><Button variant="ghost" size="sm" @click="editCompany(company)">Edit</Button></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No companies yet. Add your first company below.</p>
              <div v-if="props.canWrite" class="settings-grid">
                <div class="settings-field"><Label for="ledger-company-name">Code</Label><Input id="ledger-company-name" v-model="companyForm.name" placeholder="acme" /><p class="settings-note">Short unique code. Never changes.</p></div>
                <div class="settings-field"><Label for="ledger-company-label">Name</Label><Input id="ledger-company-label" v-model="companyForm.label" placeholder="Acme" /></div>
                <div class="settings-field"><Label for="ledger-company-base">Main money</Label><Input id="ledger-company-base" v-model="companyForm.baseCurrency" placeholder="USD" /><p class="settings-note">The money this company reports in.</p></div>
                <div class="settings-field"><Label for="ledger-company-disabled">Inactive</Label><Checkbox id="ledger-company-disabled" :model-value="companyForm.disabled" @update:model-value="companyForm.disabled = $event === true" /><p class="settings-note">Inactive items stay for history but can't be used.</p></div>
              </div>
              <div v-if="props.canWrite" class="ledger-actions"><Button size="sm" @click="saveCompany">Save company</Button></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Branches</CardTitle><CardDescription>Places or teams inside {{ currentCompany || 'the company' }}.</CardDescription></CardHeader>
            <CardContent>
              <Table v-if="branches.length > 0">
                <TableHeader><TableRow><TableHead>Code</TableHead><TableHead>Belongs under</TableHead><TableHead>Group</TableHead><TableHead></TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="branch in branches" :key="branch.name"><TableCell><code>{{ branch.name }}</code></TableCell><TableCell>{{ branch.parent || '-' }}</TableCell><TableCell>{{ branch.isGroup ? 'yes' : 'no' }}</TableCell><TableCell><Button variant="ghost" size="sm" @click="editBranch(branch)">Edit</Button></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No branches yet.</p>
              <div v-if="props.canWrite && hasCompany" class="settings-grid">
                <div class="settings-field"><Label for="ledger-branch-name">Code</Label><Input id="ledger-branch-name" v-model="branchForm.name" /></div>
                <div class="settings-field"><Label for="ledger-branch-label">Name</Label><Input id="ledger-branch-label" v-model="branchForm.label" /></div>
                <div class="settings-field"><Label for="ledger-branch-parent">Belongs under</Label><Input id="ledger-branch-parent" v-model="branchForm.parent" /><p class="settings-note">A group branch above this one, if any.</p></div>
                <div class="settings-field"><Label for="ledger-branch-group">This is a group</Label><Checkbox id="ledger-branch-group" :model-value="branchForm.isGroup" @update:model-value="branchForm.isGroup = $event === true" /><p class="settings-note">Groups organize. Only the rest can take postings.</p></div>
              </div>
              <div v-if="props.canWrite && hasCompany" class="ledger-actions"><Button size="sm" @click="saveBranch">Save branch</Button></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Departments</CardTitle><CardDescription>Who spends and earns inside {{ currentCompany || 'the company' }}.</CardDescription></CardHeader>
            <CardContent>
              <Table v-if="departments.length > 0">
                <TableHeader><TableRow><TableHead>Code</TableHead><TableHead>Branch</TableHead><TableHead>Belongs under</TableHead><TableHead></TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="department in departments" :key="department.name"><TableCell><code>{{ department.name }}</code></TableCell><TableCell>{{ department.branch || '-' }}</TableCell><TableCell>{{ department.parent || '-' }}</TableCell><TableCell><Button variant="ghost" size="sm" @click="editDepartment(department)">Edit</Button></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No departments yet.</p>
              <div v-if="props.canWrite && hasCompany" class="settings-grid">
                <div class="settings-field"><Label for="ledger-department-name">Code</Label><Input id="ledger-department-name" v-model="departmentForm.name" /></div>
                <div class="settings-field"><Label for="ledger-department-label">Name</Label><Input id="ledger-department-label" v-model="departmentForm.label" /></div>
                <div class="settings-field"><Label for="ledger-department-branch">Branch</Label><Input id="ledger-department-branch" v-model="departmentForm.branch" /><p class="settings-note">Which branch this sits in, if any.</p></div>
                <div class="settings-field"><Label for="ledger-department-parent">Belongs under</Label><Input id="ledger-department-parent" v-model="departmentForm.parent" /></div>
                <div class="settings-field"><Label for="ledger-department-group">This is a group</Label><Checkbox id="ledger-department-group" :model-value="departmentForm.isGroup" @update:model-value="departmentForm.isGroup = $event === true" /><p class="settings-note">Groups organize. Only the rest can take postings.</p></div>
              </div>
              <div v-if="props.canWrite && hasCompany" class="ledger-actions"><Button size="sm" @click="saveDepartment">Save department</Button></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Cost centres</CardTitle><CardDescription>What costs are tracked against.</CardDescription></CardHeader>
            <CardContent>
              <Table v-if="costCentres.length > 0">
                <TableHeader><TableRow><TableHead>Code</TableHead><TableHead>Belongs under</TableHead><TableHead>Group</TableHead><TableHead></TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="centre in costCentres" :key="centre.name"><TableCell><code>{{ centre.name }}</code></TableCell><TableCell>{{ centre.parent || '-' }}</TableCell><TableCell>{{ centre.isGroup ? 'yes' : 'no' }}</TableCell><TableCell><Button variant="ghost" size="sm" @click="editCostCentre(centre)">Edit</Button></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No cost centres yet.</p>
              <div v-if="props.canWrite && hasCompany" class="settings-grid">
                <div class="settings-field"><Label for="ledger-cc-name">Code</Label><Input id="ledger-cc-name" v-model="costCentreForm.name" /></div>
                <div class="settings-field"><Label for="ledger-cc-label">Name</Label><Input id="ledger-cc-label" v-model="costCentreForm.label" /></div>
                <div class="settings-field"><Label for="ledger-cc-parent">Belongs under</Label><Input id="ledger-cc-parent" v-model="costCentreForm.parent" /></div>
                <div class="settings-field"><Label for="ledger-cc-group">This is a group</Label><Checkbox id="ledger-cc-group" :model-value="costCentreForm.isGroup" @update:model-value="costCentreForm.isGroup = $event === true" /><p class="settings-note">Groups organize. Only the rest can take postings.</p></div>
              </div>
              <div v-if="props.canWrite && hasCompany" class="ledger-actions"><Button size="sm" @click="saveCostCentre">Save cost centre</Button></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Dimensions</CardTitle><CardDescription>Extra labels a posting can carry, like project or region.</CardDescription></CardHeader>
            <CardContent>
              <Table v-if="dimensions.length > 0">
                <TableHeader><TableRow><TableHead>Code</TableHead><TableHead>Allowed values</TableHead><TableHead>Always needed for</TableHead><TableHead></TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="dimension in dimensions" :key="dimension.name"><TableCell><code>{{ dimension.name }}</code></TableCell><TableCell>{{ dimension.members.join(', ') }}</TableCell><TableCell>{{ dimension.mandatoryFor.join(', ') || '-' }}</TableCell><TableCell><Button variant="ghost" size="sm" @click="editDimension(dimension)">Edit</Button></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No dimensions yet.</p>
              <div v-if="props.canWrite" class="settings-grid">
                <div class="settings-field"><Label for="ledger-dimension-name">Code</Label><Input id="ledger-dimension-name" v-model="dimensionForm.name" /></div>
                <div class="settings-field"><Label for="ledger-dimension-label">Name</Label><Input id="ledger-dimension-label" v-model="dimensionForm.label" /></div>
                <div class="settings-field"><Label for="ledger-dimension-members">Allowed values</Label><Input id="ledger-dimension-members" v-model="dimensionForm.members" /><p class="settings-note">Comma separated, e.g. website,retail.</p></div>
                <div class="settings-field"><Label for="ledger-dimension-mandatory">Always needed for</Label><Input id="ledger-dimension-mandatory" v-model="dimensionForm.mandatoryFor" /><p class="settings-note">Accounts whose postings must pick a value, comma separated.</p></div>
                <div class="settings-field"><Label for="ledger-dimension-from">Valid from</Label><Input id="ledger-dimension-from" v-model="dimensionForm.validFrom" placeholder="YYYY-MM-DD" /><p class="settings-note">Leave empty for always.</p></div>
                <div class="settings-field"><Label for="ledger-dimension-to">Valid to</Label><Input id="ledger-dimension-to" v-model="dimensionForm.validTo" placeholder="YYYY-MM-DD" /></div>
              </div>
              <div v-if="props.canWrite"><Button size="sm" @click="saveDimension">Save dimension</Button></div>
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
                <div class="settings-field"><Label for="ledger-book">Book</Label><Input id="ledger-book" v-model="book" placeholder="main" @change="loadPeriods" /><p class="settings-note">Which set of books. Usually main.</p></div>
              </div>
              <Table v-if="fiscalYears.length > 0">
                <TableHeader><TableRow><TableHead>Code</TableHead><TableHead>Dates</TableHead><TableHead></TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="year in fiscalYears" :key="year.name"><TableCell><code>{{ year.name }}</code></TableCell><TableCell>{{ year.startDate }}..{{ year.endDate }}</TableCell><TableCell><Button variant="ghost" size="sm" @click="editFiscalYear(year)">Edit</Button></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No financial years yet.</p>
              <div v-if="props.canWrite && hasCompany" class="settings-grid">
                <div class="settings-field"><Label for="ledger-fy-name">Code</Label><Input id="ledger-fy-name" v-model="fiscalYearForm.name" /></div>
                <div class="settings-field"><Label for="ledger-fy-label">Name</Label><Input id="ledger-fy-label" v-model="fiscalYearForm.label" /></div>
                <div class="settings-field"><Label for="ledger-fy-start">First day</Label><Input id="ledger-fy-start" v-model="fiscalYearForm.startDate" placeholder="YYYY-MM-DD" /></div>
                <div class="settings-field"><Label for="ledger-fy-end">Last day</Label><Input id="ledger-fy-end" v-model="fiscalYearForm.endDate" placeholder="YYYY-MM-DD" /></div>
              </div>
              <div v-if="props.canWrite && hasCompany" class="ledger-actions"><Button size="sm" @click="saveFiscalYear">Save financial year</Button></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Periods</CardTitle><CardDescription>Posting windows. Dates never overlap within one book. Close a period to stop new postings; a hard close cannot be undone here.</CardDescription></CardHeader>
            <CardContent>
              <Table v-if="periods.length > 0">
                <TableHeader><TableRow><TableHead>Code</TableHead><TableHead>Dates</TableHead><TableHead>Status</TableHead><TableHead></TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="period in periods" :key="period.name"><TableCell><code>{{ period.name }}</code></TableCell><TableCell>{{ period.startDate }}..{{ period.endDate }}</TableCell><TableCell><Badge :variant="period.status === 'open' ? 'secondary' : 'outline'">{{ period.status }}</Badge></TableCell><TableCell><Button variant="ghost" size="sm" @click="editPeriod(period)">Edit</Button> <Button v-if="props.canWrite && period.status === 'open'" variant="ghost" size="sm" @click="setPeriodStatus(period, 'soft_closed')">Close</Button> <Button v-if="props.canWrite && period.status === 'soft_closed'" variant="ghost" size="sm" @click="setPeriodStatus(period, 'open')">Reopen</Button> <Button v-if="props.canWrite && period.status !== 'hard_closed'" variant="ghost" size="sm" @click="setPeriodStatus(period, 'hard_closed')">Hard close</Button></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No periods yet.</p>
              <div v-if="props.canWrite && hasCompany" class="settings-grid">
                <div class="settings-field"><Label for="ledger-period-name">Code</Label><Input id="ledger-period-name" v-model="periodForm.name" /></div>
                <div class="settings-field"><Label for="ledger-period-label">Name</Label><Input id="ledger-period-label" v-model="periodForm.label" /></div>
                <div class="settings-field"><Label for="ledger-period-fy">Financial year</Label><Input id="ledger-period-fy" v-model="periodForm.fiscalYear" /></div>
                <div class="settings-field"><Label for="ledger-period-start">First day</Label><Input id="ledger-period-start" v-model="periodForm.startDate" placeholder="YYYY-MM-DD" /></div>
                <div class="settings-field"><Label for="ledger-period-end">Last day</Label><Input id="ledger-period-end" v-model="periodForm.endDate" placeholder="YYYY-MM-DD" /></div>
              </div>
              <div v-if="props.canWrite && hasCompany" class="ledger-actions"><Button size="sm" @click="savePeriod">Save period</Button></div>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
      <TabsContent value="currencies">
        <div class="ledger-stack">
          <Card>
            <CardHeader><CardTitle>Currencies</CardTitle><CardDescription>Money the books can use.</CardDescription></CardHeader>
            <CardContent>
              <Table v-if="currencies.length > 0">
                <TableHeader><TableRow><TableHead>Code</TableHead><TableHead>Name</TableHead><TableHead>Decimal places</TableHead><TableHead></TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="currency in currencies" :key="currency.code"><TableCell><code>{{ currency.code }}</code></TableCell><TableCell>{{ currency.label }}</TableCell><TableCell>{{ currency.exponent }}</TableCell><TableCell><Button variant="ghost" size="sm" @click="editCurrency(currency)">Edit</Button></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No currencies yet.</p>
              <div v-if="props.canWrite" class="settings-grid">
                <div class="settings-field"><Label for="ledger-currency-code">Code</Label><Input id="ledger-currency-code" v-model="currencyForm.code" placeholder="USD" /><p class="settings-note">Three letters, e.g. USD.</p></div>
                <div class="settings-field"><Label for="ledger-currency-label">Name</Label><Input id="ledger-currency-label" v-model="currencyForm.label" /></div>
                <div class="settings-field"><Label for="ledger-currency-exponent">Decimal places</Label><Input id="ledger-currency-exponent" :model-value="String(currencyForm.exponent)" @update:model-value="currencyForm.exponent = Number($event)" /><p class="settings-note">Digits after the point: 2 for most money.</p></div>
                <div class="settings-field"><Label for="ledger-currency-disabled">Inactive</Label><Checkbox id="ledger-currency-disabled" :model-value="currencyForm.disabled" @update:model-value="currencyForm.disabled = $event === true" /></div>
              </div>
              <div v-if="props.canWrite"><Button size="sm" @click="saveCurrency">Save currency</Button></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Exchange rates</CardTitle><CardDescription>What one money is worth in another, from a moment on. Rates are never edited; save a new one instead.</CardDescription></CardHeader>
            <CardContent>
              <div class="settings-grid">
                <div class="settings-field"><Label for="ledger-rate-from">From</Label><Input id="ledger-rate-from" v-model="rateForm.from" placeholder="USD" /></div>
                <div class="settings-field"><Label for="ledger-rate-to">To</Label><Input id="ledger-rate-to" v-model="rateForm.to" placeholder="EUR" /></div>
                <div class="settings-field"><Label for="ledger-rate-type">Kind of rate</Label><Input id="ledger-rate-type" v-model="rateForm.rateType" /><p class="settings-note">spot, average…</p></div>
                <div class="settings-field"><Label for="ledger-rate-source">Source</Label><Input id="ledger-rate-source" v-model="rateForm.source" /><p class="settings-note">Where the rate came from.</p></div>
                <div class="settings-field"><Label for="ledger-rate-at">Starting</Label><Input id="ledger-rate-at" v-model="rateForm.at" placeholder="2026-01-01T00:00:00Z" /><p class="settings-note">Date and time, e.g. 2026-01-01T00:00:00Z.</p></div>
                <div class="settings-field"><Label for="ledger-rate-rate">Rate</Label><Input id="ledger-rate-rate" v-model="rateForm.rate" /></div>
              </div>
              <div class="ledger-actions"><Button v-if="props.canWrite" size="sm" @click="saveRate">Save rate</Button> <Button size="sm" variant="outline" @click="lookupRate">Look up</Button> <code v-if="lookedUpRate">{{ lookedUpRate }}</code></div>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
      <TabsContent value="accounts">
        <div class="ledger-stack">
          <Card>
            <CardHeader><CardTitle>Account types</CardTitle><CardDescription>Kinds of account, like asset or income.</CardDescription></CardHeader>
            <CardContent>
              <Table v-if="accountTypes.length > 0">
                <TableHeader><TableRow><TableHead>Code</TableHead><TableHead>Usual balance</TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="type in accountTypes" :key="type.name"><TableCell><code>{{ type.name }}</code></TableCell><TableCell>{{ type.normalBalance }}</TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No account types yet.</p>
              <div v-if="props.canWrite" class="settings-grid">
                <div class="settings-field"><Label for="ledger-type-name">Code</Label><Input id="ledger-type-name" v-model="accountTypeForm.name" /></div>
                <div class="settings-field"><Label for="ledger-type-label">Name</Label><Input id="ledger-type-label" v-model="accountTypeForm.label" /></div>
                <div class="settings-field"><Label for="ledger-type-balance">Usual balance</Label><Input id="ledger-type-balance" v-model="accountTypeForm.normalBalance" placeholder="debit" /><p class="settings-note">Which side balances usually sit on: debit or credit.</p></div>
              </div>
              <div v-if="props.canWrite" class="ledger-actions"><Button size="sm" @click="saveAccountType">Save account type</Button></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Accounts</CardTitle><CardDescription>The chart of accounts. Groups organize; only the rest can take postings.</CardDescription></CardHeader>
            <CardContent>
              <Table v-if="accounts.length > 0">
                <TableHeader><TableRow><TableHead>Code</TableHead><TableHead>Type</TableHead><TableHead>Belongs under</TableHead><TableHead>Group</TableHead><TableHead></TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="account in accounts" :key="account.name"><TableCell><code>{{ account.name }}</code></TableCell><TableCell>{{ account.type }}</TableCell><TableCell>{{ account.parent || '-' }}</TableCell><TableCell>{{ account.isGroup ? 'yes' : 'no' }}</TableCell><TableCell><Button variant="ghost" size="sm" @click="editAccount(account)">Edit</Button></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No accounts yet.</p>
              <div v-if="props.canWrite" class="settings-grid">
                <div class="settings-field"><Label for="ledger-account-name">Code</Label><Input id="ledger-account-name" v-model="accountForm.name" /></div>
                <div class="settings-field"><Label for="ledger-account-label">Name</Label><Input id="ledger-account-label" v-model="accountForm.label" /></div>
                <div class="settings-field"><Label for="ledger-account-type">Type</Label><Input id="ledger-account-type" v-model="accountForm.type" /></div>
                <div class="settings-field"><Label for="ledger-account-parent">Belongs under</Label><Input id="ledger-account-parent" v-model="accountForm.parent" /><p class="settings-note">A group account above this one, if any.</p></div>
                <div class="settings-field"><Label for="ledger-account-group">This is a group</Label><Checkbox id="ledger-account-group" :model-value="accountForm.isGroup" @update:model-value="accountForm.isGroup = $event === true" /><p class="settings-note">Groups organize. Only the rest can take postings.</p></div>
                <div class="settings-field"><Label for="ledger-account-class">Class</Label><Input id="ledger-account-class" v-model="accountForm.class" /><p class="settings-note">Optional grouping used by reports.</p></div>
                <div class="settings-field"><Label for="ledger-account-role">Controls</Label><Input id="ledger-account-role" v-model="accountForm.controlRole" /><p class="settings-note">What this account controls, if anything.</p></div>
                <div class="settings-field"><Label for="ledger-account-company">Company</Label><Input id="ledger-account-company" v-model="accountForm.company" /><p class="settings-note">Empty means every company.</p></div>
              </div>
              <div v-if="props.canWrite"><Button size="sm" @click="saveAccount">Save account</Button></div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle>Posting rules</CardTitle><CardDescription>Decide which account a posting uses. More detail wins; ties are refused.</CardDescription></CardHeader>
            <CardContent>
              <Table v-if="determinations.length > 0">
                <TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Account</TableHead><TableHead>Matches when</TableHead><TableHead></TableHead></TableRow></TableHeader>
                <TableBody><TableRow v-for="determination in determinations" :key="determination.id"><TableCell>{{ determination.label }}</TableCell><TableCell><code>{{ determination.account }}</code></TableCell><TableCell>{{ Object.entries(determination.match).map(([k, v]) => `${k}=${v}`).join(', ') || 'always' }}</TableCell><TableCell><Button variant="ghost" size="sm" @click="editDetermination(determination)">Edit</Button></TableCell></TableRow></TableBody>
              </Table>
              <p v-else class="empty-state">No posting rules yet.</p>
              <div v-if="props.canWrite" class="settings-grid">
                <div class="settings-field"><Label for="ledger-det-label">Name</Label><Input id="ledger-det-label" v-model="determinationForm.label" /><p class="settings-note">A name you'll recognize.</p></div>
                <div class="settings-field"><Label for="ledger-det-account">Account</Label><Input id="ledger-det-account" v-model="determinationForm.account" /></div>
                <div class="settings-field"><Label for="ledger-det-match">Matches when</Label><Input id="ledger-det-match" v-model="determinationForm.match" /><p class="settings-note">Posting details, e.g. company=acme. Empty matches everything.</p></div>
              </div>
              <div v-if="props.canWrite"><Button size="sm" @click="saveDetermination">Save posting rule</Button></div>
              <div class="settings-grid">
                <div class="settings-field"><Label for="ledger-resolve-context">Posting details</Label><Input id="ledger-resolve-context" v-model="resolveContext" /><p class="settings-note">Try a rule: details like company=acme, then Check.</p></div>
              </div>
              <div class="ledger-actions"><Button size="sm" variant="outline" @click="resolvePreview">Check</Button> <code v-if="resolvedAccount">Uses account: {{ resolvedAccount }}</code></div>
            </CardContent>
          </Card>
        </div>
      </TabsContent>
    </Tabs>
  </div>
</template>
