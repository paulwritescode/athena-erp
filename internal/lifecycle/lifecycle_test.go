package lifecycle

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestNextAllowsOnlyTheDocumentedTransitions(t *testing.T) {
	allowed := map[[2]any]State{
		{Draft, Submit}:     Submitted,
		{Submitted, Cancel}: Cancelled,
		{Cancelled, Amend}:  Draft,
	}
	for _, from := range []State{Draft, Submitted, Cancelled} {
		for _, action := range []Action{Submit, Cancel, Amend} {
			to, err := Next(from, action)
			want, ok := allowed[[2]any{from, action}]
			if ok && (err != nil || to != want) {
				t.Errorf("%s %s = %s, %v; want %s", from, action, to, err, want)
			}
			if !ok && !errors.Is(err, ErrTransition) {
				t.Errorf("%s %s should be refused, got %s, %v", from, action, to, err)
			}
		}
	}
}

// Every sequence lists its events in the relative order of spec 5.2, with the
// in-transaction steps before the steps after the commit.
func TestSequencesFollowTheFixedOrder(t *testing.T) {
	for _, action := range []Action{SaveNew, SaveEdit, Submit, Cancel, Amend, Trash} {
		position, afterCommit := -1, false
		for _, step := range Sequence(action) {
			index := slices.Index(Order, step.Name)
			if index < 0 {
				t.Fatalf("%s: %s is not in Order", action, step.Name)
			}
			// on_status_change closes the in-transaction group, so it sorts last in Order.
			if step.Name != OnStatus && step.Name != AfterCommit && index <= position {
				t.Errorf("%s: %s breaks the order", action, step.Name)
			}
			if step.Name != OnStatus {
				position = max(position, index)
			}
			if !step.InTx {
				afterCommit = true
			} else if afterCommit {
				t.Errorf("%s: %s runs in the transaction after a post-commit step", action, step.Name)
			}
		}
	}
}

func TestRegistryRunsHooksInOrderForTheMatchingRecordType(t *testing.T) {
	registry := NewRegistry()
	var calls []string
	registry.On(OnSubmit, "", func(context.Context, *Event) error { calls = append(calls, "all"); return nil })
	registry.On(OnSubmit, "order", func(context.Context, *Event) error { calls = append(calls, "order"); return nil })
	registry.On(OnSubmit, "other", func(context.Context, *Event) error { calls = append(calls, "other"); return nil })
	if err := registry.Fire(context.Background(), Event{Name: OnSubmit, RecordType: "order"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"all", "order"}) {
		t.Fatalf("calls = %v", calls)
	}
	stop := errors.New("stop")
	registry.On(OnSubmit, "order", func(context.Context, *Event) error { return stop })
	registry.On(OnSubmit, "order", func(context.Context, *Event) error { calls = append(calls, "late"); return nil })
	if err := registry.Fire(context.Background(), Event{Name: OnSubmit, RecordType: "order"}); !errors.Is(err, stop) || slices.Contains(calls, "late") {
		t.Fatalf("an error must stop the later hooks: %v %v", err, calls)
	}
	var none *Registry
	if err := none.Fire(context.Background(), Event{Name: OnSubmit}); err != nil {
		t.Fatal(err)
	}
}
