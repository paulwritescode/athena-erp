// Package lifecycle holds the document state machine and the event registry
// of the kernel (spec 5.1 and 5.2). It knows nothing about storage: the
// server runs a transition, fires the events in the fixed order below and
// hands each hook the open transaction when the event runs inside it.
package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

// State is the document state the kernel owns. Business status belongs to
// Packs and is not modelled here.
type State int32

const (
	Draft     State = 0
	Submitted State = 1
	Cancelled State = 2
)

func (s State) String() string {
	switch s {
	case Draft:
		return "draft"
	case Submitted:
		return "submitted"
	case Cancelled:
		return "cancelled"
	}
	return fmt.Sprintf("state(%d)", int32(s))
}

// Action is a transition a caller may ask for.
type Action string

const (
	Submit Action = "submit"
	Cancel Action = "cancel"
	Amend  Action = "amend"
)

// ErrTransition reports an action the current state does not allow.
var ErrTransition = errors.New("transition not allowed")

// Next returns the state a document reaches by action. Amend does not change
// the original: it needs a cancelled document and creates a new draft, so Next
// returns the state of the successor.
func Next(from State, action Action) (State, error) {
	switch {
	case action == Submit && from == Draft:
		return Submitted, nil
	case action == Cancel && from == Submitted:
		return Cancelled, nil
	case action == Amend && from == Cancelled:
		return Draft, nil
	}
	return from, fmt.Errorf("%w: a %s document cannot be %s", ErrTransition, from, pastTense(action))
}

func pastTense(action Action) string {
	switch action {
	case Submit:
		return "submitted"
	case Cancel:
		return "cancelled"
	}
	return "amended"
}

// Name identifies an event (spec 5.2).
type Name string

const (
	BeforeInsert   Name = "before_insert"
	BeforeValidate Name = "before_validate"
	Validate       Name = "validate"
	BeforeSave     Name = "before_save"
	BeforeSubmit   Name = "before_submit"
	OnSubmit       Name = "on_submit"
	AfterSubmit    Name = "after_submit"
	AfterCommit    Name = "after_commit"
	BeforeCancel   Name = "before_cancel"
	OnCancel       Name = "on_cancel"
	AfterCancel    Name = "after_cancel"
	OnAmend        Name = "on_amend"
	OnTrash        Name = "on_trash"
	OnStatus       Name = "on_status_change"
)

// Order is the fixed event order of spec 5.2. A transition fires the events
// it uses in this relative order. on_link and on_unlink arrive with the
// phases that add document links.
var Order = []Name{BeforeInsert, BeforeValidate, Validate, BeforeSave, BeforeSubmit, OnSubmit, AfterSubmit, AfterCommit, BeforeCancel, OnCancel, AfterCancel, OnAmend, OnTrash, OnStatus}

// Step is one event of a transition. InTx events run inside the transaction
// that changes the document, so an error rolls everything back. The others
// run after the commit and cannot undo it.
type Step struct {
	Name Name
	InTx bool
}

// Sequence returns the events a transition fires, in firing order. The
// in-transaction steps come first, then the steps after the commit.
// SaveNew and SaveEdit stand for creating and updating a draft.
func Sequence(action Action) []Step {
	switch action {
	case SaveNew:
		return []Step{{BeforeInsert, true}, {BeforeValidate, true}, {Validate, true}, {BeforeSave, true}}
	case SaveEdit:
		return []Step{{BeforeValidate, true}, {Validate, true}, {BeforeSave, true}}
	case Submit:
		return []Step{{BeforeSubmit, true}, {OnSubmit, true}, {OnStatus, true}, {AfterSubmit, false}, {AfterCommit, false}}
	case Cancel:
		return []Step{{BeforeCancel, true}, {OnCancel, true}, {OnStatus, true}, {AfterCancel, false}, {AfterCommit, false}}
	case Amend:
		return []Step{{OnAmend, true}, {AfterCommit, false}}
	case Trash:
		return []Step{{OnTrash, true}, {AfterCommit, false}}
	}
	return nil
}

// Pseudo-actions that name the draft saves and the draft delete.
const (
	SaveNew  Action = "save_new"
	SaveEdit Action = "save_edit"
	Trash    Action = "trash"
)

// Event is what a hook receives. Tx is set for in-transaction events and nil
// after the commit. Hooks must not retain it.
type Event struct {
	Name       Name
	RecordType string
	RecordID   string
	// RelatedID is the original on amend; empty otherwise.
	RelatedID string
	RequestID string
	// Actor, Workspace and Revision feed the durable event envelope; the
	// server sets them on transitions that emit one.
	Actor     string
	Workspace string
	Revision  int64
	From, To  State
	Tx        *sql.Tx
	// Doc is the document the event is about, when it has one in hand.
	Doc *Document
}

// Document is a document as the kernel holds it during an event, in the
// stored (database argument) form: nil, bool, string, or int64 minor units
// for a Money field. A hook changes it with Set; the kernel applies those
// writes after the event, through the same checks as a user's save.
type Document struct {
	// Values holds every field by key.
	Values map[string]any
	// System holds the kernel columns (revision, docstate, owner and so on).
	System map[string]any
	// Writes holds what hooks set, by field key.
	Writes map[string]any
	order  []string
}

// Set records a field change. Later hooks see the new value.
func (d *Document) Set(key string, value any) {
	if d.Writes == nil {
		d.Writes = map[string]any{}
	}
	if _, seen := d.Writes[key]; !seen {
		d.order = append(d.order, key)
	}
	d.Writes[key] = value
	d.Values[key] = value
}

// Written returns the keys hooks set, in the order they first set them.
func (d *Document) Written() []string { return append([]string(nil), d.order...) }

// Hook reacts to an event. An error from an in-transaction hook rejects the
// operation and rolls it back.
type Hook func(ctx context.Context, e *Event) error

type registration struct {
	recordType string
	hook       Hook
}

// Registry holds the hooks. A nil Registry has none. Starlark Rules attach to
// the same events through one hook and never bypass these checks.
type Registry struct {
	mu    sync.RWMutex
	hooks map[Name][]registration
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{hooks: map[Name][]registration{}} }

// On registers hook for an event of one Record Type, or of every Record Type
// when recordType is empty. Hooks run in registration order.
func (r *Registry) On(name Name, recordType string, hook Hook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[name] = append(r.hooks[name], registration{recordType, hook})
}

// Fire runs the hooks for e.Name and stops at the first error.
func (r *Registry) Fire(ctx context.Context, e Event) error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	registered := append([]registration(nil), r.hooks[e.Name]...)
	r.mu.RUnlock()
	for _, item := range registered {
		if item.recordType != "" && item.recordType != e.RecordType {
			continue
		}
		if err := item.hook(ctx, &e); err != nil {
			return err
		}
	}
	return nil
}
