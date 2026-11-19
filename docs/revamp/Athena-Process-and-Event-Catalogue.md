# Athena ERP • Processes, movements and events

20 November 2026 · Proposed design and coverage audit · [ERP playbook](Athena-ERP-Playbook.md) · [Movement and posting examples](Records-and-movements.md) · [Industry workflows](Athena-Industry-and-Operations-Design.md) · [Checkout, commerce and learning](Athena-Checkout-Commerce-and-Learning.md)

**Athena already describes what moves. The missing layer is a contract for what makes it move.** Consider the last cake at a supermarket. A customer confirms an order. Athena reserves one cake. A provider confirms the deposit. A picker puts the cake in a labelled box. A cashier hands it over. These facts happen at different times and can arrive in a different order. Treating them all as “order updated” makes it possible to tell a customer their cake is ready before anyone has packed it, or issue stock twice when a payment callback is repeated.

## 1. Coverage audit

| Existing source | What is already captured | What this catalogue adds |
|---|---|---|
| Records and movements, “Movement coverage by business cycle” and numerical fixtures | Document chains, quantity/value movements, accounts affected, reconciliation and reversals across trading, production and finance. | Initiator, authoritative commit, automatic follow-up, explicit human decision and owner of an exception. |
| ERP playbook, “One engine from button to books” | Command IDs, revisions, transaction boundaries, posting plan, transactional outbox, idempotency and bounded Starlark Rules. | How a Pack declares the business fact it produces and what a subscriber is allowed to do with it. |
| Industry design and checkout volumes | Detailed journeys for workers, stores, schools and factories; scheduled and provider-driven actions appear within individual examples. | Cross-Pack causality, races between actors/timers/providers, wait states, recovery and user-visible explanations. |
| Go checkout inspected at HEAD `7985ef8`, `internal/lifecycle/lifecycle.go` | Submit runs `before_submit`, `on_submit` and `on_status_change` in the transaction, then `after_submit` and `after_commit`. Cancel has a corresponding sequence. Draft saves have their own hook sequence. | A lifecycle hook marks a point in execution. A durable business event identifies a committed fact another process can consume after a restart. The latter and the Pack bindings below are design targets. This was a source review, with no runtime certification. |

## 2. The contract for one transition

| Question a Pack author must answer | Example from receiving goods |
|---|---|
| **Who can initiate it?** Name an authorised person, service, authenticated external observation or scheduled job. Specify command, expected revision, scope and prerequisites. | Receiving Clerk submits a Purchase Receipt for 6 accepted units against a particular PO line. An authorised import can request the same command with its own identity. |
| **What fact becomes true?** Give the fact one owning Pack, a past-tense name, a stable schema and a business identity. | The receipt line has been accepted and posted. Its event is tied to that voucher revision and line. |
| **What must succeed together?** Specify state before/after, line applications, ledger deltas, audit and emitted fact. | Under the perpetual-stock policy, enter 6 units in stock, Dr Stock 600 / Cr GRNI 600, apply 6 units to the PO line and store an outbox entry in the same transaction. All roll back together on failure. |
| **What can happen after commit?** Name each subscriber, its purpose, actor, effect key and failure owner. | Buying refreshes an expected-receipt projection. A matching worker reevaluates pending supplier bills. Optional notifications enqueue privately. An email outage leaves the posted receipt valid. |
| **Where must a person still decide?** Provide the eligible Role, deadline, evidence, fallback and compensation. | A bill for 8 while only 6 are accepted creates a variance. The AP reviewer can seek a correction or approve a documented policy exception within their authority. A successful receipt never independently releases supplier payment. |

**Four kinds of movement.** Physical movement changes quantity, custody, location or disposition. Financial movement changes money, liability, valuation or settlement. Entitlement movement changes a reservation, leave balance, approved earning, award or permission. Workflow movement changes responsibility, approval or readiness. A transfer to a quarantine location can remove stock from sale while preserving its total value. Sending a label to a printer has an operational outcome but creates no stock or GL movement.

**How Go and Starlark cooperate.** Go validates authority, expected revision, scope, period, holds and invariants. Inside the transaction a deterministic Rule can reject, derive values or propose a posting plan. Go applies the accepted plan and persists the event with the business change. After commit, a worker runs a declared reaction or opens a task; any new write goes through an authorised command in a new transaction. A schedule requests a command in exactly the same way. A provider webhook first enters a verified inbox; a callback is evidence to evaluate. Raw provider data, a frontend click or a delivered notification cannot independently establish a paid, inspected or released state.

**Which actions can be automatic?** Encode defaults per transition. A deterministic stock issue from an approved dispatch runs automatically in the posting transaction. A reversible planning suggestion can automatically create a draft. A payment, shared credential change, write-off, quality release or grade publication needs the declared authority and evidence. A company may delegate well-defined actions to a service principal under a versioned policy with amount/scope limits, explicit exclusions, expiry and audit. Editing a Rule must never expand its actor’s Grants. If no eligible approver is available, surface the exception and escalation deadline.

## 3. Worked process • reserve, pay, pick and collect

The proposed baseline below uses an outlet with perpetual stock, a payment-before-handover option and an order deposit. The sales invoice recognises revenue at handover under this fixture. Real deployments choose their documented recognition and fiscal policies. Payment, picking and collection keep independent state so an event arriving early cannot imply the other two.

| Initiator and command or observation | Committed fact / movement | Automatic reaction | Wait, exception or recovery |
|---|---|---|---|
| Shopper confirms a branch cart | Order accepted with price snapshot, fulfilment mode and source lines. For the last cake, allocate the hold atomically or return unavailable. Available-to-promise falls by 1; on-hand stays unchanged. | Queue ticket generation. Under staff-pick policy queue one pick task when its start conditions hold. Record the reservation deadline. | Two competing carts cannot both get a confirmed hold. In a shopper-picked order skip the staff pick task. |
| Shopper selects M-PESA; cashier may initiate a counter tender | Create a Tender Attempt for the unpaid amount, with operation ID, merchant and expected amount. | A worker initiates the authorised provider request and records its reference. | Unknown initiation is a reconciliation case. Retrying the UI command must not send another debit. |
| Provider result, scheduled status check or cashier’s **Check payment** | Ingest evidence. After merchant, identity, amount and currency validation, allocate confirmed funds once. For the prepaid fixture Dr Wallet Clearing / Cr Customer Advances. | Update balance due and receipt status. Deliver a payment receipt through the outbox. If prepayment gates picking, release the pick task once. | A timeout or unverified SMS leaves the attempt unresolved. A second genuine transfer goes to unapplied funds. A paid order can still be awaiting picking. |
| Picker claims and scans actual goods | Pick task records actual variant, quantity/weight and container. A physical mispick is visible even at the same price. | Compare with order lines. Calculate variance or propose substitution. | Additional cost needs agreement and tender for the difference. A short pick does not silently modify an already issued receipt. |
| Picker confirms all containers staged | Fulfilment is ready after quantities, substitutions and locations are resolved. | Queue one ready notification per fulfilment revision. Reprint a failed label using the same container ID. | Customer can collect with a valid claim reference even if email delivery failed. Moving a bag updates its staging location. |
| Cashier scans customer code and all containers or actual goods, verifies balance, then selects **Complete handover** | Lock claim, order and relevant allocations. Record one handover, consume reservation, issue stock and redeem claim once under the chosen posting owner. For a prepaid sale apply deposit to invoice. | Queue final receipt, customer timeline and outlet projections. | Two tills scanning the same ticket see one handover. A lost response retrieves the committed result. A counter-payment order waits for confirmed tender before release. |
| Scheduler detects expired hold or collection window | Check expected state and revision under lock. Expire eligible unpaid hold and release availability. A paid, perishable no-show opens a disposition task under outlet policy. | Refresh listings and notify staff where permitted. | A late successful payment is real money. If stock was released, route to availability review, reschedule or linked refund. Do not silently recreate the hold. |
| Customer cancels or returns goods | Before dispatch, release eligible reservation; after dispatch, inspect returned goods. Refund, credit and restock have separate source events and controls. | Dispatch approved refund request; reconcile provider outcome independently. | A food-safety hold survives a financial refund. A failed refund stays a liability/case for finance until resolved. |

**Numeric check.** One KES 1,800 cake with KES 900 carrying cost and zero tax. At reservation, on-hand remains 1 and available becomes 0. Confirming payment moves 1,800 into wallet clearing against customer advances; revenue and COGS remain zero. At handover issue 1 cake, Dr COGS 900 / Cr Stock 900. Raise the invoice Dr Receivable 1,800 / Cr Revenue 1,800 and apply Dr Customer Advances 1,800 / Cr Receivable 1,800. Closing reservation, on-hand, advance and receivable are all zero. A delayed callback, repeated ticket scan or receipt reprint cannot change those balances. A direct-sale Pack may use a combined voucher if it designates a single owner for each leg.

## 4. Process register across the existing Packs

Read each row left to right. A scheduled check can discover that work is due; authority for the resulting change still comes from a Workflow, Grant and effective policy. Journal directions are illustrative where stated. The linked volumes retain field-level and accounting detail.

### Trading, warehouse and accounting

| Trigger / initiator | Owning transition and movement | Automatic next step | Human gate or exception |
|---|---|---|---|
| Sales User revises quotation; customer accepts | Freeze accepted version; create order under the configured confirmation policy. Order may reserve stock/exposure; quotation produces no GL. | Recalculate open quantities and proposed fulfilment. | Credit limit or price deviation routes to an approver; rejected/expired offer releases any temporary hold. |
| Storekeeper confirms delivery; billing clerk or enabled policy invoices it | Delivery owns dispatch and cost. Sales Invoice owns revenue/tax/receivable on its declared event. Applications link exact source lines. | Update open-to-deliver/open-to-bill by line; issue invoice notification after commit. | Partial deliveries stay open. A combined invoice may own dispatch only for quantities not already issued. |
| Buyer submits requisition; approver releases PO | Commit approved budget/quantity under policy; preserve supplier, prices and terms. | Issue PO and create expected receipts. | Threshold, vendor or segregation conflict waits for authorised reviewer. An overdue PO prompts follow-up, never automatic receipt. |
| Receiver accepts delivery or service manager accepts work | Accepted goods increase owned stock and receipt accrual per policy; service acceptance records evidence without inventory. | Re-evaluate receipt-based match by PO line. | Quarantine/rejected units retain custody and disposition; AP sees the unmatched residual. |
| AP clerk or integration submits invoice | Record validated liability or hold draft under policy. Matched stock charges clear GRNI; invoice variances stay identifiable. | Schedule due-date proposal, surface duplicate supplier references. | Payment release requires treasury authority. Imported invoice approval is separate from bank transfer. |
| Treasury releases payment; provider or bank confirms | Approved instruction, transfer outcome, accounting entry, settlement application and statement match each retain separate evidence. | Reconcile known references and generate outstanding-item report. | Unknown outcome → query original transfer; unmatched charges, ambiguous matches and write-offs require review. |
| Storekeeper dispatches and destination confirms receipt | Transfer source → transit → destination. Partial receipt leaves residual in transit. | Update availability by location and alert on overdue residual. | Report loss/damage separately. A timer cannot certify arrival. |
| Counter submits count at cutoff | Record observed quantity; calculate variance allowing for intervening movements. | Draft adjustment or route threshold exception. | Approved adjustment posts delta; never overwrite current stock with an old absolute count. |
| Quality reviewer flags or releases lot; operator reports scrap | Hold/release changes saleability; approved destruction posts quantity and cost. | Traverse genealogy, withdraw affected listings and open recall tasks. | Named reviewer releases or disposes; identify unresolved downstream recipients. Accidental hold release gets a linked corrective disposition. |
| Stock drops below reorder threshold or planning job runs | Recompute demand and dated supplies. Create a deduplicated replenishment proposal. | Assign buyer/planner, link demand origins. | Buyer approves PO or Work Order; received stock, budget and commitments remain distinct. |
| Finance approves freight, FX correction, credit or return | Allocate cost-only deltas to remaining stock and consumed cost; credits and physical returns have independent postings. | Refresh valuation/ageing and report reconciliation state. | Closed periods need authorised correction date; previous published reports stay reproducible. |
| Employee submits expense; supervisor approves | Apply approved expense, tax and advance applications, accrue residual payable. | Include residual in payment proposal and flag missing receipts. | Treasury releases actual payment. Company-card items cannot also reimburse the employee. |
| Period/contract deadline arrives | Draft recurring invoice, accrual, reversal, deferral release or remeasurement under the relevant schedule. | Post only if policy delegates it and prerequisites still pass; link schedule occurrence to result. | Missing rate or locked period blocks that occurrence; recovery honours prior completed occurrences. |
| Controller requests close or consolidation | Check subledger controls, in-flight valuations, bank reconciliation and intercompany pairs; acquire posting barrier and seal eligible book/period. | Produce Closing Pack and authorised Report Runs at a common cutoff. | Unexplained residual, unmatched intercompany leg or required missing sign-off remains open. Reopen/reclose is separately authorised and versioned. |

### Work, assets, production and food

| Trigger / initiator | Owning transition and movement | Automatic next step | Human gate or exception |
|---|---|---|---|
| Foreman publishes roster; worker checks in/out; supervisor records site quantities | Planned assignment, actual attendance, claimed output and evidence are distinct facts. | Detect overlap or missing checkout; queue measurement review. | Missing photo triggers alternative evidence review. It cannot automatically invalidate time wages. |
| Surveyor certifies work; payroll cutoff arrives | Accepted quantity can feed a contract claim and eligible piece earning through separate allocations. Approved time feeds hourly earnings. | Calculate proposed earnings at dated rates; flag duplicates, disputed units and deadline risk. | Payroll approves run, treasury releases cash, bank confirms. Client retention has no automatic effect on wages. |
| Finance accepts capital cost; asset accountant approves partial commissioning | Eligible cost enters CIP; commissioning transfers only allocated ready-for-use components to the asset book. | Generate depreciation schedule using service date, life, residual and book revision. | No depreciation on residual CIP. Late costs require review; custody/location transfer alone creates no depreciation expense. |
| Depreciation schedule comes due; technician submits meter reading | Due occurrence can post approved book depreciation or create a draft. Meter may trigger a maintenance task. | Update asset register, service due list and book reconciliation. | Reporting-only book creates no statutory GL. A service reminder cannot certify completed maintenance. |
| Planner releases Work Order; operator issues materials and reports output | Freeze BOM/routing; post source lots to WIP; production confirms accepted output, by-products and residual WIP. | Calculate yield, variance, traceability and proposed rework. | Quality releases held output; cost accountant resolves residual. Manual issue and backflush cannot consume the same input twice. |
| Chef records prep; kitchen confirms a made-to-order item | Prep changes raw stock into intermediate stock. Selected fulfilment event consumes raw components or prepared portions per recipe. | Update available portions and theoretical food cost. | Comp, cancellation after cooking and spoilage need dispositions. Refund does not put a consumed meal back on the shelf. |
| Dairy collector records weighing; quality reviewer accepts source lot | Provisional observation → accepted payable quantity and pool contribution. Rejected litres retain source identity. | Apply effective rate and premium, build producer statement, propagate approved tank hold to descendants. | Rate dispute creates adjustment. Settlement approval and payout are separate; accepted lot alone cannot initiate bank payment. |
| House operator records placement, transfer, losses and graded eggs | Population roll-forward and egg lot disposition; approved hatchery transfer consumes only allocated eggs. | Update yields using declared denominators and open count discrepancy. | Unknown classification remains visible. Hatching/dispatch requires observation and counts; elapsed incubation time merely prompts work. |
| Cutter or processor confirms input/output weights and package lot | Consume identified input lots, allocate outputs, loss and cost, preserve all source-to-output links. | Generate labels, yield report and trace index. | Release requires applicable inspection. Reprinting labels cannot increase pack count. |
| HR approves leave, employment change or termination | Effective-dated assignment and leave entitlement change; payroll applies approved time and pay policy. | Recompute future proposals and expire linked access on effective date. | Retroactive payroll changes use adjustment runs. A change of job title alone cannot grant unrestricted system access. |

### Channels, learning and platform operations

| Trigger / initiator | Owning transition and movement | Automatic next step | Human gate or exception |
|---|---|---|---|
| Cashier scans, weighs or parks; shopper requests kiosk help | Accept basket line or create lane assistance task. Basket entry moves no cash; reservation only under declared policy. | Refresh estimate and notify assigned attendant. | Price dispute, restricted item, scale error or handoff requires scoped resolution. |
| Merchant publishes listing; marketplace sends order | Publish approved projection; deduplicate authorised inbound order and allocate branch availability. | Push availability revisions and create fulfilment work according to pick mode. | Rejected listing, oversell or expired price remains owned by merchandising/fulfilment. |
| Shopper comments or replies; WhatsApp Flow completes | Comment creates purchase intent under channel policy. Validated, unexpired response requests only its bound command. | Offer permitted private checkout, update permitted source Record or open agent task. | Customer confirms purchase/substitution. No comment, read receipt or Flow completion alone establishes payment or consent to further contact. |
| Instructor publishes lesson; learner enrols or submits assessment | Release authorised course version; record enrolment, immutable final submission and receipt. | Score eligible questions, queue manual marking, schedule reminders under consent. | Deadline references server time/accommodations. Manual marking and moderation remain with eligible staff. |
| Moderator approves marks; registrar issues transcript | Release result revision; snapshot authorised grades and issuer evidence. | Deliver private availability notice; correct a prior issue by supersession. | Regrade is reviewed; repeated issuance command cannot award credits twice. |
| Administrator invites, grants, revokes or resets | Change membership/recovery state and policy revision atomically with audit. | Invalidate affected caches/sessions as policy requires; queue invite or reset delivery. | Protected ownership, delegated authority and last-owner rules apply even to imports and jobs. |
| Owner publishes Settings or Pack upgrade | Validate dependencies, capabilities, irreversible schema steps and Grant diff; pin exact revision. | Invalidate relevant caches and discover affected future schedules. | Stop on conflicting movement owners, unapproved privilege expansion or incompatible pending event schema. |
| Report schedule, retention deadline, credential rotation or failed delivery occurs | Schedule creates one logical job. Report Run freezes cutoff; retention evaluates holds; rotation tracks version; delivery updates attempt status. | Retry only safe operations, alert owner on exhaustion; audit resolution. | Recheck recipient access before delivery. Legal hold blocks disposal. Restoring backups must reconcile external effects before re-enabling outgoing workers. |

## 5. What a Pack must declare

Keep the existing names: a **Workflow** defines legal transitions and who can take them; a **Rule** is Starlark code bound to a declared hook or reaction; a **Grant** supplies authority; a **Document** records a business action; a **Ledger** records an actual movement. Extend Pack metadata with event and subscription declarations. Operational delivery state can live in Core-owned tables. A new top-level Kind or a separate message broker is unnecessary for the first version.

**Example binding, proposed syntax only.** A collection order may be packed before the funds arrive, while an outlet policy may require prepayment before picking. Configure each gate independently. The example sends readiness notices only when packing is complete.

```yaml
event: fulfilment.collection_ready
schema_version: 1
producer: fulfilment
subject: collection_order
transition: confirm_staging
requires:
  - all_required_containers_staged
  - all_pick_exceptions_resolved
commit:
  - record_fulfilment_revision
  - append_audit
  - enqueue_domain_event
subscriptions:
  - consumer: notifications.pickup_ready
    mode: after_commit
    action: enqueue_ready_notice
    effect_key: [workspace_id, fulfilment_id, ready_revision, recipient_id, channel]
    on_exhausted: assign_delivery_exception
```

One authoritative command determines readiness. If picking is allowed only after prepayment, the pick-task gate listens to both order acceptance and payment confirmation, rereads authoritative state and creates **one** task when both conditions hold. This join works in either arrival order. It also illustrates why writing two independent “create task” Rules would be wrong.

| Runtime contract | Fields and guarantees |
|---|---|
| Event definition and envelope | Stable namespaced type, schema version, immutable event ID, Workspace, producer, subject Type/ID/revision, actor/service, command operation ID, correlation and immediate causation IDs, recorded sequence, occurred/recorded times, business date if relevant, minimal typed payload. Preserve source-line IDs on movement events. External adapters may map these fields to CloudEvents for exchange. [CloudEvents specification](https://github.com/cloudevents/spec/blob/main/cloudevents/spec.md) |
| Workflow instance and task | Correlate related Records under one business process ID. Store definition/version, current phase, expected facts, active wait, due time, assigned Role/Scope, completion evidence, attempts, reason and linked correction. Long waits survive service restart. A user can inspect which condition is still missing. |
| Producer transaction and consumer inbox | With MariaDB, store the business write, Ledger effects and outbox row in one transaction. On consumption, record consumer/event or business effect key and the local effect together. A worker may wake on a callback or poll durable rows. Recovery scans durable state after crashes. [Transactional outbox guidance](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html) |
| Ordering, retries and compensation | Order matters per affected subject/line, not globally. Use revisions and lock competing transitions. A stale notification cannot move a paid order back to pending. Gap-dependent consumers wait or reconcile from source. Retry transient failures with backoff; assign permanent failures to an owner. Corrections are new linked actions with their own approvals and posting dates. |
| External effects | Printing, email, banks, fiscal gateways and messaging have independent acceptance semantics. Use provider idempotency and status lookup where available. After ambiguous delivery, query or present “unknown”; a successful local commit cannot guarantee one physical print, one email or one external debit. A reprint carries its copy marker. |
| Replay, evolution and Pack ownership | Rebuild derived views from a cutoff with a rebuild-only identity; disable external sends and business command execution during projection replay. Historical corrections use explicit commands. Pin the Workflow/Rule version on in-flight work; upcast or migrate older event payloads before removing support. Prevent unbounded reaction cycles through declared dependencies, causation checks, budgets and an operator-visible stop state. |
| Permission, privacy and policy | Labour/education/payment events carry minimal identifiers and scoped references. A subscriber cannot inherit the producer's entire access. Apply the appropriate delegated authority, check current grants for new actions and current recipient rights on delivery. Redact restricted payloads from the activity view. |
| Schedule semantics | Give each logical occurrence a unique key over Workspace, schedule, business scope and period. Track timezone, cutoff, calendar, missed-run/catch-up policy, lease, watermark and result. On expiry, recheck the subject under lock. A job cannot retroactively expire completed work or repeat financial posting after restart. |

### How operators see and control this

- **Build → Workflows.** Each transition shows *Started by*, *Conditions*, *Changes on commit*, *Runs afterwards*, *Who approves*, *Deadline* and *If it fails*. Preview on a synthetic Record. Show policy version, required Grants, event consumers and predicted movements before publishing.
- **View → Record → Activity.** Show a chronological business trail with actor labels such as “Customer”, “Receiving Clerk”, “Provider confirmation”, “Scheduled check” and “Rule”. For the cake: **Payment confirmed → waiting for picker → all containers staged → ready notice queued → collected at counter**. Only display states backed by evidence. Show sibling steps where order varies.
- **Work queues.** A picker sees work they can claim. A manager sees approvals approaching cutoff. Finance sees unresolved payments and reconciliations. Each task has an owner, age, next action and source Record. **Retry delivery**, **Check provider status**, **Approve exception** and **Reverse posting** are separately permissioned commands with different effects.
- **Explain this movement.** From a Report amount or stock quantity, follow Ledger row → voucher and source line → transition → Rule and config revisions → originating person, timer or provider evidence → correction, if any. A technical request ID alone cannot explain a business process spanning several days.

## 6. Acceptance gates and outstanding policy

| Failure or race to exercise | Expected observable result |
|---|---|
| Database rejects the second leg of a receipt posting | No accepted receipt, GL leg, stock increase or publishable event. A rejected attempt is identifiable separately. |
| Server stops after commit and before dispatch; two workers later consume the event | The committed movement survives, its outbox is recoverable, and each consumer records at most one local business effect. |
| Expiry worker and verified payment compete for the last cake | A locked, current reservation determines the winner. Confirmed money remains recorded exactly once. Unfulfillable payment enters an owned resolution case. |
| Picker double-clicks **Ready**, email times out and customer collects anyway | One ready transition, recorded delivery status and one handover. An uncertain email cannot prevent release of a properly verified order. |
| Bill and receipt arrive in either order; a supplier uses identical Items on two PO lines | Match exact line applications, respect tolerance and preserve all unapplied amounts. No duplicate bill or automatic release of money. |
| Pay-run scheduler restarts mid-batch; two graders release the same corrected result | Wage item IDs and release revisions de-duplicate work. Paid workers stay paid once; prior transcript stays superseded and credits accrue once. |
| Physical return precedes financial refund, or vice versa | Two independent statuses and responsible parties remain visible. Closed periods, quality holds and original-cost reversal are respected. |
| Projection rebuild replays historic receipt, payment and notification events | Derived balances/reports tie to source at the watermark. No new sales, payment requests, messages, awards or bank releases. |

**Coverage boundary.** This register covers the major process families in the current Athena designs. Before a Pack ships, expand *every* enabled transition into the contract in section 2, including cancellation, reassignment, partial completion and recovery. A Pack author still needs the operator’s authority matrix, timeouts, country/book rules, choice of posting owner, privacy/retention policy and provider confirmation semantics. Flag a missing transition or policy in setup rather than silently supplying an irreversible default. The first executable proof should be the prepaid collection journey above plus the purchase-receipt fixture, each exercised through the actual desk and Go services under crash and retry conditions.
