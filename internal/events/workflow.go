package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"athena-erp/internal/store"

	"github.com/google/uuid"
)

// Instance statuses of an athena_workflow_instances row. done and
// budget_exhausted are terminal: transitions on a terminal instance are
// no-ops, so redelivered signals can never reopen it.
const (
	InstanceStatusOpen            = "open"
	InstanceStatusDone            = "done"
	InstanceStatusBudgetExhausted = "budget_exhausted"
)

// Task statuses of an athena_tasks row.
const (
	TaskStatusOpen = "open"
	TaskStatusDone = "done"
)

// ConsumerWorkflows is the dispatcher consumer name that wakes workflow
// waits. The server registers WorkflowHandler under it; every event type
// an instance waits on needs a subscription for this consumer.
const ConsumerWorkflows = "workflows"

// DefaultCycleBudget caps the reactions on one instance when its
// definition names none. It matches the migration 17 column default.
const DefaultCycleBudget = 25

// DefaultPhase is the phase of a started instance the caller does not name.
const DefaultPhase = "running"

var (
	// ErrInstanceNotFound reports a signal, wait or completion for an
	// unknown workflow instance.
	ErrInstanceNotFound = errors.New("events: workflow instance not found")
	// ErrTaskNotFound reports a completion for an unknown workflow task.
	ErrTaskNotFound = errors.New("events: workflow task not found")
)

// Definition is one workflow version: the gates whose facts Signal
// collects and the cycle budget the instance may spend. The definition
// travels with the instance row, so an instance always evaluates the
// version it started with.
type Definition struct {
	Name string `json:"name"`
	// Version numbers the definition; zero starts at 1.
	Version int `json:"version"`
	// CycleBudget caps the instance's reactions; zero or less starts at
	// DefaultCycleBudget.
	CycleBudget int    `json:"cycle_budget"`
	Gates       []Gate `json:"gates"`
}

// Gate creates one task, keyed by TaskKey, once every fact in Needs has
// arrived. A gate with no needs creates its task when the instance
// starts. Role and Scope default to the instance's assignment when empty.
type Gate struct {
	TaskKey string   `json:"task_key"`
	Title   string   `json:"title"`
	Needs   []string `json:"needs,omitempty"`
	Role    string   `json:"role,omitempty"`
	Scope   string   `json:"scope,omitempty"`
}

// StartOptions carries the per-instance fields of R8.7 beyond the
// definition: the phase, due time and assignment.
type StartOptions struct {
	// Phase names the starting step; empty means DefaultPhase.
	Phase string
	// DueAt is the instance deadline; nil means none.
	DueAt *time.Time
	// Role and Scope assign the instance; empty means unassigned.
	Role  string
	Scope string
}

// Instance is one row of athena_workflow_instances: its definition and
// phase, the facts it expects and has received, its wait, due time and
// assignment, its spent and declared cycle budget, and, once finished,
// its completion evidence.
type Instance struct {
	ID            string
	Workflow      string
	Version       int
	Definition    Definition
	Phase         string
	Status        string
	ExpectedFacts []string
	ReceivedFacts []string
	WaitEvent     string
	WaitUntil     *time.Time
	DueAt         *time.Time
	Role          string
	Scope         string
	CycleCount    int
	CycleBudget   int
	Evidence      map[string]any
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Task is one row of athena_tasks: a gated unit of work of one instance,
// unique by instance plus task key so two conditions racing still create
// it once.
type Task struct {
	ID          string
	InstanceID  string
	TaskKey     string
	Title       string
	Status      string
	Role        string
	Scope       string
	DueAt       *time.Time
	CompletedAt *time.Time
	Evidence    map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SignalOutcome is what one signal did: the tasks it created, or Stopped
// when the signal would have raised the instance past its cycle budget.
// A stop is a recorded outcome, not a failure: the instance row carries
// the budget_exhausted status, and retrying can never succeed, so the
// caller commits the transaction either way.
type SignalOutcome struct {
	Created []Task
	Stopped bool
}

// DueWait is an open instance whose wait deadline has passed. Waits are
// durable rows, so they survive restarts; the operator or a worker lists
// them here and completes or re-waits the instance.
type DueWait struct {
	InstanceID string
	Workflow   string
	WaitEvent  string
	WaitUntil  time.Time
}

// Start stores an open instance of the definition in the caller's
// transaction and returns its id. Gates with no needs create their task
// at once. The expected facts are the sorted union of the gates' needs.
func Start(ctx context.Context, tx *sql.Tx, def Definition, opts StartOptions) (string, error) {
	if def.Name == "" {
		return "", fmt.Errorf("events: a workflow needs a name")
	}
	version := def.Version
	if version <= 0 {
		version = 1
	}
	budget := def.CycleBudget
	if budget <= 0 {
		budget = DefaultCycleBudget
	}
	seen := map[string]bool{}
	var expected []string
	for i, gate := range def.Gates {
		if gate.TaskKey == "" {
			return "", fmt.Errorf("events: gate %d needs a task key", i)
		}
		if seen[gate.TaskKey] {
			return "", fmt.Errorf("events: gate %q is declared twice", gate.TaskKey)
		}
		seen[gate.TaskKey] = true
		if gate.Title == "" {
			return "", fmt.Errorf("events: gate %q needs a title", gate.TaskKey)
		}
		for _, need := range gate.Needs {
			if need == "" {
				return "", fmt.Errorf("events: gate %q needs non-empty facts", gate.TaskKey)
			}
		}
		expected = append(expected, gate.Needs...)
	}
	sort.Strings(expected)
	expected = dedupeStrings(expected)
	stored := Definition{Name: def.Name, Version: version, CycleBudget: budget, Gates: def.Gates}
	encodedDef, err := json.Marshal(stored)
	if err != nil {
		return "", err
	}
	encodedExpected, err := json.Marshal(nonNilStrings(expected))
	if err != nil {
		return "", err
	}
	phase := opts.Phase
	if phase == "" {
		phase = DefaultPhase
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	instance := id.String()
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_workflow_instances
		(id, workflow_name, definition_version, definition, phase, status,
		 expected_facts, received_facts, due_at, assigned_role, assigned_scope,
		 cycle_count, cycle_budget)
		VALUES (?, ?, ?, ?, ?, 'open', ?, '[]', ?, ?, ?, 0, ?)`,
		instance, def.Name, version, string(encodedDef), phase,
		string(encodedExpected), nullTime(opts.DueAt),
		nullText(opts.Role), nullText(opts.Scope), budget); err != nil {
		return "", fmt.Errorf("events: start instance: %w", err)
	}
	for _, gate := range def.Gates {
		if len(gate.Needs) == 0 {
			if _, err := insertTask(ctx, tx, instance, gate, opts.Role, opts.Scope); err != nil {
				return "", err
			}
		}
	}
	return instance, nil
}

// Signal records one fact on an open instance in the caller's
// transaction: it spends one cycle, appends the fact, and creates every
// newly satisfied gate's task exactly once. A repeated fact costs no
// cycle; a signal on a terminal instance is a no-op. When the signal
// would raise the instance past its cycle budget, Signal stops it
// instead: the row keeps the budget_exhausted status with the stopped
// fact as evidence, and the outcome reports Stopped.
func Signal(ctx context.Context, tx *sql.Tx, classifier Classifier, instanceID, fact string) (SignalOutcome, error) {
	if instanceID == "" || fact == "" {
		return SignalOutcome{}, fmt.Errorf("events: a signal needs an instance and a fact")
	}
	inst, err := loadInstance(ctx, tx, instanceID, true)
	if err != nil {
		return SignalOutcome{}, err
	}
	return signalLocked(ctx, tx, classifier, inst, fact)
}

// signalLocked runs Signal on a row the caller locked.
func signalLocked(ctx context.Context, tx *sql.Tx, classifier Classifier, inst Instance, fact string) (SignalOutcome, error) {
	if inst.Status != InstanceStatusOpen {
		return SignalOutcome{}, nil
	}
	for _, received := range inst.ReceivedFacts {
		if received == fact {
			return SignalOutcome{}, nil
		}
	}
	if inst.CycleCount+1 > inst.CycleBudget {
		evidence, err := json.Marshal(map[string]any{
			"stopped_fact": fact,
			"cycle_count":  inst.CycleCount,
			"cycle_budget": inst.CycleBudget,
		})
		if err != nil {
			return SignalOutcome{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE athena_workflow_instances
			SET status = 'budget_exhausted', evidence = ? WHERE id = ?`,
			string(evidence), inst.ID); err != nil {
			return SignalOutcome{}, fmt.Errorf("events: stop instance: %w", err)
		}
		return SignalOutcome{Stopped: true}, nil
	}
	received := append(inst.ReceivedFacts, fact)
	encoded, err := json.Marshal(received)
	if err != nil {
		return SignalOutcome{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE athena_workflow_instances
		SET cycle_count = cycle_count + 1, received_facts = ? WHERE id = ?`,
		string(encoded), inst.ID); err != nil {
		return SignalOutcome{}, fmt.Errorf("events: record fact: %w", err)
	}
	have := make(map[string]bool, len(received))
	for _, name := range received {
		have[name] = true
	}
	var created []Task
	for _, gate := range inst.Definition.Gates {
		satisfied := true
		for _, need := range gate.Needs {
			if !have[need] {
				satisfied = false
				break
			}
		}
		if !satisfied {
			continue
		}
		task, err := insertTaskOnce(ctx, tx, classifier, inst, gate)
		if err != nil {
			return SignalOutcome{}, err
		}
		if task != nil {
			created = append(created, *task)
		}
	}
	return SignalOutcome{Created: created}, nil
}

// WorkflowHandler wakes the waits on each delivered event. The server
// registers it under ConsumerWorkflows; an event contributes its type as
// a fact to every open instance waiting on it, so gates name event types
// alongside manual facts.
func WorkflowHandler(classifier Classifier) Handler {
	return func(ctx context.Context, tx *sql.Tx, event Envelope) error {
		_, err := WakeWaits(ctx, tx, classifier, event.EventType)
		return err
	}
}

// WakeWaits clears every open wait on the event type in the caller's
// transaction and signals each woken instance with the event type as its
// fact. It reports how many instances it woke. Instances lock in id
// order, so concurrent wakes cannot deadlock.
func WakeWaits(ctx context.Context, tx *sql.Tx, classifier Classifier, eventType string) (int, error) {
	if eventType == "" {
		return 0, fmt.Errorf("events: waking waits needs an event type")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM athena_workflow_instances
		WHERE status = 'open' AND wait_event = ? ORDER BY id FOR UPDATE`, eventType)
	if err != nil {
		return 0, fmt.Errorf("events: read waits: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("events: scan wait: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("events: read waits: %w", err)
	}
	rows.Close()
	for _, id := range ids {
		inst, err := loadInstance(ctx, tx, id, false)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE athena_workflow_instances
			SET wait_event = NULL, wait_until = NULL WHERE id = ?`, id); err != nil {
			return 0, fmt.Errorf("events: clear wait: %w", err)
		}
		inst.WaitEvent = ""
		inst.WaitUntil = nil
		if _, err := signalLocked(ctx, tx, classifier, inst, eventType); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// WaitFor parks an open instance on an event until a deadline, in the
// caller's transaction. The matching event wakes it through WakeWaits;
// the deadline shows it in DueWaits. Waiting on a terminal instance is
// a no-op.
func WaitFor(ctx context.Context, tx *sql.Tx, instanceID, event string, until time.Time) error {
	if instanceID == "" || event == "" {
		return fmt.Errorf("events: a wait needs an instance and an event")
	}
	if until.IsZero() {
		return fmt.Errorf("events: a wait needs a deadline")
	}
	result, err := tx.ExecContext(ctx, `UPDATE athena_workflow_instances
		SET wait_event = ?, wait_until = ? WHERE id = ? AND status = 'open'`,
		event, until.UTC(), instanceID)
	if err != nil {
		return fmt.Errorf("events: wait: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("events: wait: %w", err)
	}
	if affected > 0 {
		return nil
	}
	if _, err := loadInstance(ctx, tx, instanceID, false); err != nil {
		return err
	}
	return nil
}

// DueWaits lists the open instances whose wait deadline has passed, in
// deadline order. A zero or negative limit reads 100.
func DueWaits(ctx context.Context, q Querier, now time.Time, limit int) ([]DueWait, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.QueryContext(ctx, `SELECT id, workflow_name, wait_event, wait_until
		FROM athena_workflow_instances
		WHERE status = 'open' AND wait_until IS NOT NULL AND wait_until <= ?
		ORDER BY wait_until, id LIMIT ?`, now.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("events: read due waits: %w", err)
	}
	defer rows.Close()
	var out []DueWait
	for rows.Next() {
		var wait DueWait
		if err := rows.Scan(&wait.InstanceID, &wait.Workflow, &wait.WaitEvent, &wait.WaitUntil); err != nil {
			return nil, fmt.Errorf("events: scan due wait: %w", err)
		}
		wait.WaitUntil = wait.WaitUntil.UTC()
		out = append(out, wait)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("events: read due waits: %w", err)
	}
	return out, nil
}

// Complete finishes an open instance with its completion evidence, in
// the caller's transaction. Completing a terminal instance is a no-op.
func Complete(ctx context.Context, tx *sql.Tx, instanceID string, evidence map[string]any) error {
	if instanceID == "" {
		return fmt.Errorf("events: completing needs an instance")
	}
	encoded, err := json.Marshal(nonNilPayload(evidence))
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE athena_workflow_instances
		SET status = 'done', evidence = ? WHERE id = ? AND status = 'open'`,
		string(encoded), instanceID)
	if err != nil {
		return fmt.Errorf("events: complete instance: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("events: complete instance: %w", err)
	}
	if affected > 0 {
		return nil
	}
	if _, err := loadInstance(ctx, tx, instanceID, false); err != nil {
		return err
	}
	return nil
}

// CompleteTask finishes an open task with its evidence, in the caller's
// transaction. Completing a finished task is a no-op.
func CompleteTask(ctx context.Context, tx *sql.Tx, taskID string, at time.Time, evidence map[string]any) error {
	if taskID == "" {
		return fmt.Errorf("events: completing needs a task")
	}
	if at.IsZero() {
		return fmt.Errorf("events: completing needs a time")
	}
	encoded, err := json.Marshal(nonNilPayload(evidence))
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE athena_tasks
		SET status = 'done', completed_at = ?, evidence = ? WHERE id = ? AND status = 'open'`,
		at.UTC(), string(encoded), taskID)
	if err != nil {
		return fmt.Errorf("events: complete task: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("events: complete task: %w", err)
	}
	if affected > 0 {
		return nil
	}
	if _, err := loadTask(ctx, tx, taskID); err != nil {
		return err
	}
	return nil
}

// GetInstance reads one instance by id.
func GetInstance(ctx context.Context, q Querier, id string) (Instance, error) {
	return loadInstance(ctx, q, id, false)
}

// ListInstances reads instances in creation order, newest last. An empty
// status reads every status; a zero or negative limit reads 100.
func ListInstances(ctx context.Context, q Querier, status string, limit int) ([]Instance, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT ` + instanceColumns + ` FROM athena_workflow_instances`
	var args []any
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at, id LIMIT ?`
	args = append(args, limit)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("events: list instances: %w", err)
	}
	defer rows.Close()
	var out []Instance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("events: list instances: %w", err)
	}
	return out, nil
}

// GetTask reads one task by id.
func GetTask(ctx context.Context, q Querier, id string) (Task, error) {
	return loadTask(ctx, q, id)
}

// ListTasks reads one instance's tasks in creation order.
func ListTasks(ctx context.Context, q Querier, instanceID string) ([]Task, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("events: listing tasks needs an instance")
	}
	rows, err := q.QueryContext(ctx, `SELECT `+taskColumns+` FROM athena_tasks
		WHERE instance_id = ? ORDER BY created_at, id`, instanceID)
	if err != nil {
		return nil, fmt.Errorf("events: list tasks: %w", err)
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("events: list tasks: %w", err)
	}
	return out, nil
}

// insertTask stores one open task for a gate. The gate's Role and Scope
// win; empty ones fall back to the instance's assignment.
func insertTask(ctx context.Context, tx *sql.Tx, instanceID string, gate Gate, role, scope string) (Task, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Task{}, err
	}
	if gate.Role != "" {
		role = gate.Role
	}
	if gate.Scope != "" {
		scope = gate.Scope
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_tasks
		(id, instance_id, task_key, title, status, assigned_role, assigned_scope)
		VALUES (?, ?, ?, ?, 'open', ?, ?)`,
		id.String(), instanceID, gate.TaskKey, gate.Title, nullText(role), nullText(scope)); err != nil {
		return Task{}, fmt.Errorf("events: insert task: %w", err)
	}
	return loadTask(ctx, tx, id.String())
}

// insertTaskOnce stores the gate's task unless it already exists: the
// unique instance plus task key keeps two racing signals to one task. It
// reports nil when the task was already there.
func insertTaskOnce(ctx context.Context, tx *sql.Tx, classifier Classifier, inst Instance, gate Gate) (*Task, error) {
	task, err := insertTask(ctx, tx, inst.ID, gate, inst.Role, inst.Scope)
	if err == nil {
		return &task, nil
	}
	if classifier.Classify(err) == store.ErrDuplicateKey {
		return nil, nil
	}
	return nil, err
}

// instanceColumns lists every athena_workflow_instances column scanInstance
// reads.
const instanceColumns = `id, workflow_name, definition_version, CAST(definition AS CHAR),
	phase, status, CAST(expected_facts AS CHAR), CAST(received_facts AS CHAR),
	wait_event, wait_until, due_at, assigned_role, assigned_scope,
	cycle_count, cycle_budget, CAST(evidence AS CHAR), created_at, updated_at`

// loadInstance reads one instance row, locking it when lock is set.
func loadInstance(ctx context.Context, q Querier, id string, lock bool) (Instance, error) {
	query := `SELECT ` + instanceColumns + ` FROM athena_workflow_instances WHERE id = ?`
	if lock {
		query += ` FOR UPDATE`
	}
	inst, err := scanInstance(q.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Instance{}, ErrInstanceNotFound
		}
		return Instance{}, err
	}
	return inst, nil
}

// scanInstance reads an instance row. The column order must match
// instanceColumns.
func scanInstance(scanner interface {
	Scan(...any) error
}) (Instance, error) {
	var inst Instance
	var encodedDef, encodedExpected, encodedReceived string
	var waitEvent, role, scope sql.NullString
	var waitUntil, dueAt sql.NullTime
	var evidence sql.NullString
	if err := scanner.Scan(&inst.ID, &inst.Workflow, &inst.Version, &encodedDef,
		&inst.Phase, &inst.Status, &encodedExpected, &encodedReceived,
		&waitEvent, &waitUntil, &dueAt, &role, &scope,
		&inst.CycleCount, &inst.CycleBudget, &evidence,
		&inst.CreatedAt, &inst.UpdatedAt); err != nil {
		return Instance{}, err
	}
	if err := json.Unmarshal([]byte(encodedDef), &inst.Definition); err != nil {
		return Instance{}, fmt.Errorf("events: decode definition: %w", err)
	}
	var err error
	if inst.ExpectedFacts, err = decodeStrings(encodedExpected); err != nil {
		return Instance{}, fmt.Errorf("events: decode expected facts: %w", err)
	}
	if inst.ReceivedFacts, err = decodeStrings(encodedReceived); err != nil {
		return Instance{}, fmt.Errorf("events: decode received facts: %w", err)
	}
	inst.WaitEvent = waitEvent.String
	if waitUntil.Valid {
		at := waitUntil.Time.UTC()
		inst.WaitUntil = &at
	}
	if dueAt.Valid {
		at := dueAt.Time.UTC()
		inst.DueAt = &at
	}
	inst.Role = role.String
	inst.Scope = scope.String
	if evidence.Valid && evidence.String != "" {
		if err := json.Unmarshal([]byte(evidence.String), &inst.Evidence); err != nil {
			return Instance{}, fmt.Errorf("events: decode evidence: %w", err)
		}
	}
	inst.CreatedAt = inst.CreatedAt.UTC()
	inst.UpdatedAt = inst.UpdatedAt.UTC()
	return inst, nil
}

// taskColumns lists every athena_tasks column scanTask reads.
const taskColumns = `id, instance_id, task_key, title, status, assigned_role,
	assigned_scope, due_at, completed_at, CAST(evidence AS CHAR), created_at, updated_at`

// loadTask reads one task row.
func loadTask(ctx context.Context, q Querier, id string) (Task, error) {
	task, err := scanTask(q.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM athena_tasks WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, err
	}
	return task, nil
}

// scanTask reads a task row. The column order must match taskColumns.
func scanTask(scanner interface {
	Scan(...any) error
}) (Task, error) {
	var task Task
	var role, scope sql.NullString
	var dueAt, completedAt sql.NullTime
	var evidence sql.NullString
	if err := scanner.Scan(&task.ID, &task.InstanceID, &task.TaskKey, &task.Title,
		&task.Status, &role, &scope, &dueAt, &completedAt, &evidence,
		&task.CreatedAt, &task.UpdatedAt); err != nil {
		return Task{}, err
	}
	task.Role = role.String
	task.Scope = scope.String
	if dueAt.Valid {
		at := dueAt.Time.UTC()
		task.DueAt = &at
	}
	if completedAt.Valid {
		at := completedAt.Time.UTC()
		task.CompletedAt = &at
	}
	if evidence.Valid && evidence.String != "" {
		if err := json.Unmarshal([]byte(evidence.String), &task.Evidence); err != nil {
			return Task{}, fmt.Errorf("events: decode task evidence: %w", err)
		}
	}
	task.CreatedAt = task.CreatedAt.UTC()
	task.UpdatedAt = task.UpdatedAt.UTC()
	return task, nil
}

func nullTime(moment *time.Time) any {
	if moment == nil {
		return nil
	}
	return moment.UTC()
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func dedupeStrings(values []string) []string {
	out := values[:0]
	var last string
	for i, value := range values {
		if i == 0 || value != last {
			out = append(out, value)
			last = value
		}
	}
	return out
}

func decodeStrings(raw string) ([]string, error) {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []string{}, nil
	}
	return out, nil
}
