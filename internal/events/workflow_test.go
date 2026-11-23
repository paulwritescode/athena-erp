package events_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"athena-erp/internal/events"
	"athena-erp/internal/store"

	"pgregory.net/rapid"
)

func startInstance(t *testing.T, db *sql.DB, def events.Definition, opts events.StartOptions) string {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	id, err := events.Start(context.Background(), tx, def, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return id
}

func signalFact(t *testing.T, db *sql.DB, instanceID, fact string) events.SignalOutcome {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	outcome, err := events.Signal(context.Background(), tx, store.MariaDB{}, instanceID, fact)
	if err != nil {
		t.Fatal(err)
	}
	// A stop is a recorded outcome, not a failure: it commits like any
	// other signal.
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return outcome
}

func TestWorkflowStartStoresInstance(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	due := time.Date(2026, 12, 31, 12, 0, 0, 0, time.UTC)
	id := startInstance(t, db, events.Definition{
		Name: "close-month", Version: 3,
		Gates: []events.Gate{
			{TaskKey: "reconcile", Title: "Reconcile", Needs: []string{"bank", "sales"}},
			{TaskKey: "notify", Title: "Notify owner"},
		},
	}, events.StartOptions{Phase: "reconciling", DueAt: &due, Role: "System Manager", Scope: `{"company":["acme"]}`})
	inst, err := events.GetInstance(ctx, db, id)
	if err != nil {
		t.Fatal(err)
	}
	if inst.Workflow != "close-month" || inst.Version != 3 || inst.Phase != "reconciling" || inst.Status != "open" {
		t.Fatalf("instance = %+v, want close-month v3 reconciling open", inst)
	}
	if len(inst.ExpectedFacts) != 2 || inst.ExpectedFacts[0] != "bank" || inst.ExpectedFacts[1] != "sales" {
		t.Fatalf("expected = %v, want [bank sales]", inst.ExpectedFacts)
	}
	if len(inst.ReceivedFacts) != 0 {
		t.Fatalf("received = %v, want none", inst.ReceivedFacts)
	}
	if inst.DueAt == nil || !inst.DueAt.Equal(due) {
		t.Fatalf("due = %v, want %v", inst.DueAt, due)
	}
	if inst.Role != "System Manager" || inst.Scope != `{"company":["acme"]}` {
		t.Fatalf("assignment = %q %q", inst.Role, inst.Scope)
	}
	if inst.CycleCount != 0 || inst.CycleBudget != events.DefaultCycleBudget {
		t.Fatalf("cycles = %d/%d, want 0/%d", inst.CycleCount, inst.CycleBudget, events.DefaultCycleBudget)
	}
	if inst.Evidence != nil {
		t.Fatalf("evidence = %v, want nil", inst.Evidence)
	}
	// The gateless task exists from the start; the gated one does not.
	tasks, err := events.ListTasks(ctx, db, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].TaskKey != "notify" || tasks[0].Title != "Notify owner" || tasks[0].Status != "open" {
		t.Fatalf("tasks = %+v, want the notify task only", tasks)
	}
	if tasks[0].Role != "System Manager" || tasks[0].Scope != `{"company":["acme"]}` {
		t.Fatalf("task assignment = %q %q, want the instance fallback", tasks[0].Role, tasks[0].Scope)
	}
}

func TestWorkflowStartRefusesABadDefinition(t *testing.T) {
	db := setup(t)
	good := events.Gate{TaskKey: "t", Title: "T", Needs: []string{"a"}}
	for name, def := range map[string]events.Definition{
		"no name":      {Gates: []events.Gate{good}},
		"no task key":  {Name: "w", Gates: []events.Gate{{Title: "T", Needs: []string{"a"}}}},
		"dup task key": {Name: "w", Gates: []events.Gate{good, good}},
		"no title":     {Name: "w", Gates: []events.Gate{{TaskKey: "t", Needs: []string{"a"}}}},
		"empty need":   {Name: "w", Gates: []events.Gate{{TaskKey: "t", Title: "T", Needs: []string{""}}}},
	} {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := events.Start(context.Background(), tx, def, events.StartOptions{}); err == nil {
			t.Fatalf("%s: start succeeded, want an error", name)
		}
		_ = tx.Rollback()
	}
}

func TestWorkflowSignalCreatesGatedTaskInEitherOrder(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	def := events.Definition{Name: "pick-task", Gates: []events.Gate{
		{TaskKey: "pick", Title: "Pick the goods", Needs: []string{"released", "stocked"}},
	}}
	for _, order := range [][]string{{"released", "stocked"}, {"stocked", "released"}} {
		id := startInstance(t, db, def, events.StartOptions{})
		outcome := signalFact(t, db, id, order[0])
		if outcome.Stopped || len(outcome.Created) != 0 {
			t.Fatalf("order %v: first signal created %v, want nothing yet", order, outcome.Created)
		}
		outcome = signalFact(t, db, id, order[1])
		if outcome.Stopped || len(outcome.Created) != 1 || outcome.Created[0].TaskKey != "pick" {
			t.Fatalf("order %v: second signal created %+v, want the pick task", order, outcome.Created)
		}
		tasks, err := events.ListTasks(ctx, db, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 1 {
			t.Fatalf("order %v: %d tasks, want 1", order, len(tasks))
		}
		// A repeated fact costs no cycle and creates nothing.
		outcome = signalFact(t, db, id, order[0])
		if outcome.Stopped || len(outcome.Created) != 0 {
			t.Fatalf("order %v: repeat created %+v, want nothing", order, outcome.Created)
		}
		inst, err := events.GetInstance(ctx, db, id)
		if err != nil {
			t.Fatal(err)
		}
		if inst.CycleCount != 2 || len(inst.ReceivedFacts) != 2 {
			t.Fatalf("order %v: cycles %d facts %v, want 2 and 2", order, inst.CycleCount, inst.ReceivedFacts)
		}
	}
}

func TestWorkflowSignalStopsPastTheBudget(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id := startInstance(t, db, events.Definition{Name: "loopy", CycleBudget: 2, Gates: []events.Gate{
		{TaskKey: "late", Title: "Late", Needs: []string{"first", "second", "third"}},
	}}, events.StartOptions{})
	signalFact(t, db, id, "first")
	signalFact(t, db, id, "second")
	outcome := signalFact(t, db, id, "third")
	if !outcome.Stopped || len(outcome.Created) != 0 {
		t.Fatalf("third signal = %+v, want stopped with nothing created", outcome)
	}
	inst, err := events.GetInstance(ctx, db, id)
	if err != nil {
		t.Fatal(err)
	}
	if inst.Status != events.InstanceStatusBudgetExhausted {
		t.Fatalf("status = %q, want budget_exhausted", inst.Status)
	}
	if inst.Evidence["stopped_fact"] != "third" {
		t.Fatalf("evidence = %v, want the stopped fact", inst.Evidence)
	}
	if len(inst.ReceivedFacts) != 2 {
		t.Fatalf("received = %v, want the stopped fact unrecorded", inst.ReceivedFacts)
	}
	if tasks, err := events.ListTasks(ctx, db, id); err != nil || len(tasks) != 0 {
		t.Fatalf("tasks = %v, %v, want none", tasks, err)
	}
	// A stopped instance ignores further transitions.
	outcome = signalFact(t, db, id, "fourth")
	if outcome.Stopped || len(outcome.Created) != 0 {
		t.Fatalf("signal after stop = %+v, want a no-op", outcome)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := events.WaitFor(ctx, tx, store.MariaDB{}, id, "later", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("wait after stop: %v, want a no-op", err)
	}
	if err := events.Complete(ctx, tx, id, map[string]any{}); err != nil {
		t.Fatalf("complete after stop: %v, want a no-op", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if inst, err := events.GetInstance(ctx, db, id); err != nil || inst.Status != events.InstanceStatusBudgetExhausted {
		t.Fatalf("status = %q, %v, want still budget_exhausted", inst.Status, err)
	}
}

func TestWorkflowWaitWakeAndDue(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	now := time.Now().UTC().Truncate(time.Second)
	id := startInstance(t, db, events.Definition{Name: "approval", Gates: []events.Gate{
		{TaskKey: "act", Title: "Act", Needs: []string{"document.approved"}},
	}}, events.StartOptions{})
	wait := func(event string, until time.Time) {
		t.Helper()
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err := events.WaitFor(ctx, tx, store.MariaDB{}, id, event, until); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := events.DueWaits(ctx, db, now, 0); err != nil {
		t.Fatal(err)
	}
	wait("document.approved", now.Add(time.Hour))
	due, err := events.DueWaits(ctx, db, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("due now = %+v, want nothing yet", due)
	}
	due, err = events.DueWaits(ctx, db, now.Add(2*time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].InstanceID != id || due[0].WaitEvent != "document.approved" {
		t.Fatalf("due later = %+v, want the parked wait", due)
	}
	// The event wakes the wait through the dispatcher handler: the wait
	// clears, the event type lands as a fact, and the gate fires.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	handler := events.WorkflowHandler(store.MariaDB{})
	if err := handler(ctx, tx, events.Envelope{EventType: "document.approved"}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	inst, err := events.GetInstance(ctx, db, id)
	if err != nil {
		t.Fatal(err)
	}
	if inst.WaitEvent != "" || inst.WaitUntil != nil {
		t.Fatalf("wait = %q %v, want cleared", inst.WaitEvent, inst.WaitUntil)
	}
	if len(inst.ReceivedFacts) != 1 || inst.ReceivedFacts[0] != "document.approved" {
		t.Fatalf("received = %v, want the event type", inst.ReceivedFacts)
	}
	tasks, err := events.ListTasks(ctx, db, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].TaskKey != "act" {
		t.Fatalf("tasks = %+v, want the act task", tasks)
	}
}

func TestWorkflowComplete(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	now := time.Now().UTC().Truncate(time.Second)
	id := startInstance(t, db, events.Definition{Name: "done-soon", Gates: []events.Gate{
		{TaskKey: "step", Title: "Step", Needs: []string{"go"}},
	}}, events.StartOptions{})
	signalFact(t, db, id, "go")
	tasks, err := events.ListTasks(ctx, db, id)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks = %+v, %v, want one", tasks, err)
	}
	finish := func() {
		t.Helper()
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err := events.CompleteTask(ctx, tx, tasks[0].ID, now, map[string]any{"by": "ada"}); err != nil {
			t.Fatal(err)
		}
		if err := events.Complete(ctx, tx, id, map[string]any{"outcome": "shipped"}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	finish()
	// Completing twice is a no-op.
	finish()
	inst, err := events.GetInstance(ctx, db, id)
	if err != nil {
		t.Fatal(err)
	}
	if inst.Status != events.InstanceStatusDone || inst.Evidence["outcome"] != "shipped" {
		t.Fatalf("instance = %+v, want done with evidence", inst)
	}
	task, err := events.GetTask(ctx, db, tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != events.TaskStatusDone || task.CompletedAt == nil || !task.CompletedAt.Equal(now) || task.Evidence["by"] != "ada" {
		t.Fatalf("task = %+v, want done with time and evidence", task)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := events.Complete(ctx, tx, "missing-instance", map[string]any{}); !errors.Is(err, events.ErrInstanceNotFound) {
		t.Fatalf("complete missing = %v, want ErrInstanceNotFound", err)
	}
	if err := events.CompleteTask(ctx, tx, "missing-task", now, map[string]any{}); !errors.Is(err, events.ErrTaskNotFound) {
		t.Fatalf("complete missing task = %v, want ErrTaskNotFound", err)
	}
	if _, err := events.Signal(ctx, tx, store.MariaDB{}, "missing-instance", "x"); !errors.Is(err, events.ErrInstanceNotFound) {
		t.Fatalf("signal missing = %v, want ErrInstanceNotFound", err)
	}
}

func TestWorkflowLists(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	def := events.Definition{Name: "listed", Gates: []events.Gate{{TaskKey: "t", Title: "T"}}}
	first := startInstance(t, db, def, events.StartOptions{})
	second := startInstance(t, db, def, events.StartOptions{})
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := events.Complete(ctx, tx, first, map[string]any{}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	open, err := events.ListInstances(ctx, db, events.InstanceStatusOpen, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].ID != second {
		t.Fatalf("open = %+v, want the second instance only", open)
	}
	all, err := events.ListInstances(ctx, db, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all = %d instances, want 2", len(all))
	}
}

// TestGatedTaskCreationProperty checks Property 14 over drawn gates and
// arrival orders: every gate whose needs all arrive gets exactly one
// task, in any arrival order and under concurrent repeat signals, and
// gates with unmet needs get none. Repeats cost no cycle.
//
// Feature: athena-batch-2, Property 14: Gated task creation.
func TestGatedTaskCreationProperty(t *testing.T) {
	db := setup(t)
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		for _, table := range []string{"athena_tasks", "athena_workflow_instances"} {
			if _, err := db.Exec(`DELETE FROM ` + table); err != nil {
				t.Fatal(err)
			}
		}
		facts := []string{"alpha", "beta", "gamma"}
		gateCount := rapid.IntRange(1, 2).Draw(t, "gates")
		var gates []events.Gate
		for i := 0; i < gateCount; i++ {
			needCount := rapid.IntRange(1, 2).Draw(t, "needs")
			needs := map[string]bool{}
			for len(needs) < needCount {
				needs[facts[rapid.IntRange(0, len(facts)-1).Draw(t, "need")]] = true
			}
			var sorted []string
			for need := range needs {
				sorted = append(sorted, need)
			}
			gates = append(gates, events.Gate{
				TaskKey: "task-" + string(rune('a'+i)),
				Title:   "Task",
				Needs:   sorted,
			})
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		id, err := events.Start(ctx, tx, events.Definition{Name: "prop", Gates: gates}, events.StartOptions{})
		if err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		arrivals := rapid.SliceOfN(rapid.IntRange(0, len(facts)-1), 1, 6).Draw(t, "arrivals")
		racers := rapid.IntRange(1, 4).Draw(t, "racers")
		fire := func(fact string) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer tx.Rollback()
			if _, err := events.Signal(ctx, tx, store.MariaDB{}, id, fact); err != nil {
				t.Error(err)
				return
			}
			if err := tx.Commit(); err != nil {
				t.Error(err)
			}
		}
		for _, arrival := range arrivals {
			fire(facts[arrival])
		}
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, arrival := range arrivals {
					fire(facts[arrival])
				}
			}()
		}
		wg.Wait()
		if t.Failed() {
			t.FailNow()
		}
		arrived := map[string]bool{}
		for _, arrival := range arrivals {
			arrived[facts[arrival]] = true
		}
		tasks, err := events.ListTasks(ctx, db, id)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]int{}
		for _, task := range tasks {
			seen[task.TaskKey]++
		}
		for _, gate := range gates {
			satisfied := true
			for _, need := range gate.Needs {
				if !arrived[need] {
					satisfied = false
					break
				}
			}
			want := 0
			if satisfied {
				want = 1
			}
			if seen[gate.TaskKey] != want {
				t.Fatalf("gate %q needs %v, arrived %v: %d tasks, want %d",
					gate.TaskKey, gate.Needs, arrived, seen[gate.TaskKey], want)
			}
		}
		inst, err := events.GetInstance(ctx, db, id)
		if err != nil {
			t.Fatal(err)
		}
		if inst.CycleCount != len(arrived) {
			t.Fatalf("cycles = %d, distinct facts = %d, want no cycle for repeats",
				inst.CycleCount, len(arrived))
		}
	})
}

// A wait declares its own subscription, so an emitted event reaches the
// workflows consumer through the dispatcher and wakes the instance.
func TestWorkflowEventWakesWaitThroughDispatcher(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id := startInstance(t, db, events.Definition{Name: "approval", Gates: []events.Gate{
		{TaskKey: "act", Title: "Act", Needs: []string{"document.approved"}},
	}}, events.StartOptions{})
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := events.WaitFor(ctx, tx, store.MariaDB{}, id, "document.approved", time.Now().Add(time.Hour)); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	eventID := emit(t, db, events.Envelope{EventType: "document.approved", Producer: "test", SubjectType: "doc", SubjectID: "d1"})
	dispatcher := &events.Dispatcher{DB: db, Errors: store.MariaDB{}, Handlers: map[string]events.Handler{
		events.ConsumerWorkflows: events.WorkflowHandler(store.MariaDB{}),
	}}
	if ran, err := dispatcher.DispatchNext(ctx); err != nil || !ran {
		t.Fatalf("dispatch = %v, %v, want one delivery", ran, err)
	}
	if status, _ := outboxStatus(t, db, eventID); status != "dispatched" {
		t.Fatalf("outbox status = %q, want dispatched", status)
	}
	inst, err := events.GetInstance(ctx, db, id)
	if err != nil {
		t.Fatal(err)
	}
	if inst.WaitEvent != "" || len(inst.ReceivedFacts) != 1 || inst.ReceivedFacts[0] != "document.approved" {
		t.Fatalf("instance = %+v, want the wait cleared and the event recorded", inst)
	}
	tasks, err := events.ListTasks(ctx, db, id)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks = %+v, %v, want the gated task", tasks, err)
	}
}
