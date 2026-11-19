// Package rules is the Starlark host of the kernel (spec 13, ADR-0016,
// Proposed). A Rule is a small Starlark program. It runs with no I/O, no
// clock, no randomness and no floats, under step and wall-time limits, and it
// reaches data only through the host functions a Host supplies. The package
// knows nothing about storage or permissions: the kernel's Host does the
// reading as the acting user and validates every write, so a Rule can never
// do more than Go lets it.
//
// Starlark rules are not production-ready: memory is not limited (starlark-go
// has no allocation accounting), see ADR-0016.
package rules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"athena-erp/internal/types"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// Type is the kind of a Rule (spec 13.1). Posting arrives with the posting
// engine, Reaction and Scheduled with jobs.
type Type string

const (
	Validation Type = "validation"
	Derivation Type = "derivation"
	Guard      Type = "guard"
	Transform  Type = "transform"
)

// Types lists the Rule types in the order they run within one event:
// derivations first, so validations see the derived values, then guards.
var Types = []Type{Derivation, Validation, Guard, Transform}

// Valid reports whether t is a known type.
func (t Type) Valid() bool { return t.Order() >= 0 }

// Order is the run order of a type within an event, or -1 for an unknown type.
func (t Type) Order() int {
	for i, known := range Types {
		if known == t {
			return i
		}
	}
	return -1
}

// Entry is the function a Rule of this type must define.
func (t Type) Entry() string {
	switch t {
	case Validation:
		return "validate"
	case Derivation:
		return "derive"
	case Guard:
		return "guard"
	}
	return "transform"
}

// Events lists the lifecycle events a Rule of this type may attach to (spec
// 13.1). A transform has none: it runs only when called.
func (t Type) Events() []string {
	switch t {
	case Validation:
		return []string{"validate", "before_submit"}
	case Derivation:
		return []string{"validate"}
	case Guard:
		return []string{"before_submit", "before_cancel", "on_trash"}
	}
	return nil
}

// Reads reports whether the type may call get, find and exists.
func (t Type) Reads() bool { return t != Transform }

// Writes reports whether the type may call set.
func (t Type) Writes() bool { return t == Derivation }

// Limits bound one run. A Rule that reaches a limit stops with an error code
// and never hangs the server.
type Limits struct {
	// MaxSteps is the Starlark execution steps allowed.
	MaxSteps uint64
	// Timeout is the wall time allowed.
	Timeout time.Duration
	// MaxReads is the rows get, find and exists may read in total.
	MaxReads int
	// MaxWrites is the set calls allowed.
	MaxWrites int
	// MaxMessages is the warnings plus log lines kept.
	MaxMessages int
}

// DefaultLimits is what the kernel runs Rules under.
var DefaultLimits = Limits{MaxSteps: 200_000, Timeout: 500 * time.Millisecond, MaxReads: 200, MaxWrites: 50, MaxMessages: 50}

const (
	// MaxSourceBytes is the largest Rule source.
	MaxSourceBytes = 64 << 10
	maxMessageLen  = 500
)

// SourceError is a mistake in Rule source, with the position the editor shows.
type SourceError struct {
	Line, Col int
	Msg       string
}

func (e *SourceError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

// Rejection is what fail and require raise: the Rule rejects the change.
type Rejection struct {
	Code, Message string
}

func (r *Rejection) Error() string { return r.Message }

// HostError is a failure a Host reports, with a machine code. It stops the Rule
// with an error outcome.
type HostError struct {
	Code string
	Err  error
}

func (e *HostError) Error() string { return e.Err.Error() }
func (e *HostError) Unwrap() error { return e.Err }

// Refs are the names a program is checked against when it compiles, so a typo
// fails at save time with a line number rather than at the first run. A nil
// map skips that check.
type Refs struct {
	// Doc is the attributes the document has.
	Doc map[string]bool
	// Writable is the field keys set may change.
	Writable map[string]bool
}

// Program is a compiled Rule.
type Program struct {
	Type Type
	// Hash is the SHA-256 of the source, lowercase hex.
	Hash string

	prog  *starlark.Program
	entry string
}

// SourceHash returns the hash Compile records for a source.
func SourceHash(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

// fileOptions are the language options: no set, no while, no top-level
// control flow, no global reassignment and no recursion, so every program
// terminates in a bounded number of steps.
var fileOptions = &syntax.FileOptions{}

// predeclaredNames are the names a program of type t may use beyond the
// Starlark core.
func predeclaredNames(t Type) []string {
	names := []string{"fail", "warn", "require", "decimal", "money", "allocate", "round", "div", "log"}
	if t.Reads() {
		names = append(names, "get", "find", "exists")
	}
	if t.Writes() {
		names = append(names, "set")
	}
	return names
}

// Compile parses, resolves and checks source for a Rule of type t. It fails
// with a *SourceError when the program is not valid.
func Compile(t Type, source string, refs Refs) (*Program, error) {
	if !t.Valid() {
		return nil, &SourceError{Msg: fmt.Sprintf("%q is not a Rule type", t)}
	}
	if len(source) > MaxSourceBytes {
		return nil, &SourceError{Msg: fmt.Sprintf("the source is longer than %d KiB", MaxSourceBytes>>10)}
	}
	file, err := fileOptions.Parse("rule.star", source, 0)
	if err != nil {
		return nil, asSourceError(err)
	}
	known := map[string]bool{}
	for _, name := range predeclaredNames(t) {
		known[name] = true
	}
	prog, err := starlark.FileProgram(file, func(name string) bool { return known[name] })
	if err != nil {
		return nil, asSourceError(err)
	}
	if err := checkProgram(t, file, refs); err != nil {
		return nil, err
	}
	return &Program{Type: t, Hash: SourceHash(source), prog: prog, entry: t.Entry()}, nil
}

func asSourceError(err error) error {
	var syntaxErr syntax.Error
	if errors.As(err, &syntaxErr) {
		return &SourceError{Line: int(syntaxErr.Pos.Line), Col: int(syntaxErr.Pos.Col), Msg: syntaxErr.Msg}
	}
	var list resolve.ErrorList
	if errors.As(err, &list) {
		return &SourceError{Line: int(list[0].Pos.Line), Col: int(list[0].Pos.Col), Msg: list[0].Msg}
	}
	return &SourceError{Msg: err.Error()}
}

// checkProgram enforces the static rules: no floats, no division operator
// (it would yield a float), an entry function with the right shape, and known
// document attributes.
func checkProgram(t Type, file *syntax.File, refs Refs) error {
	var found error
	fail := func(pos syntax.Position, format string, args ...any) {
		if found == nil {
			found = &SourceError{Line: int(pos.Line), Col: int(pos.Col), Msg: fmt.Sprintf(format, args...)}
		}
	}
	var entry *syntax.DefStmt
	for _, stmt := range file.Stmts {
		if def, ok := stmt.(*syntax.DefStmt); ok && def.Name.Name == t.Entry() {
			entry = def
		}
	}
	if entry == nil {
		return &SourceError{Line: 1, Msg: fmt.Sprintf("a %s Rule must define def %s(doc, ctx)", t, t.Entry())}
	}
	if len(entry.Params) != 2 {
		return &SourceError{Line: int(entry.Def.Line), Col: int(entry.Def.Col), Msg: fmt.Sprintf("%s must take exactly two parameters, the document and ctx", t.Entry())}
	}
	var param string
	for i, p := range entry.Params {
		id, ok := p.(*syntax.Ident)
		if !ok {
			return &SourceError{Line: int(entry.Def.Line), Col: int(entry.Def.Col), Msg: fmt.Sprintf("%s must take two plain parameters", t.Entry())}
		}
		if i == 0 {
			param = id.Name
		}
	}
	syntax.Walk(file, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.LoadStmt:
			fail(n.Load, "load is not allowed: a Rule is one self-contained file")
		case *syntax.Literal:
			if n.Token == syntax.FLOAT {
				fail(n.TokenPos, "floats are not allowed; write decimal(%q)", n.Raw)
			}
		case *syntax.BinaryExpr:
			if n.Op == syntax.SLASH {
				fail(n.OpPos, "the / operator gives a float and is not allowed; use // for whole numbers or div(a, b, scale) for decimals")
			}
		case *syntax.AssignStmt:
			if n.Op == syntax.SLASH_EQ {
				fail(n.OpPos, "the /= operator gives a float and is not allowed")
			}
		case *syntax.Ident:
			if n.Name == "float" {
				fail(n.NamePos, "floats are not allowed")
			}
		case *syntax.DotExpr:
			if id, ok := n.X.(*syntax.Ident); ok && id.Name == param && refs.Doc != nil && !refs.Doc[n.Name.Name] {
				fail(n.Name.NamePos, "the document has no field %q", n.Name.Name)
			}
		case *syntax.CallExpr:
			if id, ok := n.Fn.(*syntax.Ident); ok && id.Name == "set" && refs.Writable != nil && len(n.Args) > 0 {
				if lit, ok := n.Args[0].(*syntax.Literal); ok && lit.Token == syntax.STRING {
					if key, _ := lit.Value.(string); !refs.Writable[key] {
						fail(lit.TokenPos, "set cannot change %q: it is not a field this Rule may set", key)
					}
				}
			}
		}
		return true
	})
	return found
}

// Status is how a run ended.
type Status string

const (
	OK       Status = "ok"
	Rejected Status = "rejected"
	Errored  Status = "error"
)

// Error codes of an errored run besides the codes a Host reports.
const (
	CodeRejected  = "rejected"
	CodeError     = "error"
	CodeStepLimit = "step_limit"
	CodeTimeLimit = "time_limit"
	CodeLimit     = "limit"
)

// Message is a warning or a log line.
type Message struct {
	Code string
	Text string
}

// Write is a field a Rule set, or one entry of a transform's result.
type Write struct {
	Field string
	Value any
}

// Row is a document or a stored Record as a Rule sees it. Values are Go
// values: nil, bool, string, int64, types.Decimal or types.Money. Hidden lists
// the fields above the acting user's permission level; reading one is an
// error.
type Row struct {
	Values map[string]any
	Hidden map[string]bool
}

// Input is everything a run may depend on. The same Input and the same
// program give the same Outcome, so a run can be replayed (spec 13.3).
type Input struct {
	Rule       string
	Version    int64
	Event      string
	RecordType string
	RecordID   string
	Doc        Row
	// Now is the injected time. A Rule never reads the clock.
	Now       time.Time
	User      string
	RequestID string
}

// Host is what the kernel supplies to a Rule. It performs every read as the
// acting user and validates every write.
type Host interface {
	// Get returns one Record the acting user may read, or false.
	Get(recordType, id string) (Row, bool, error)
	// Find returns up to limit Records equal to filters that the acting user may read.
	Find(recordType string, filters map[string]any, limit int) ([]Row, error)
	// Set changes a field of the document the Rule runs on. It fails when the
	// field is not one the Rule may change or the value is not valid.
	Set(field string, value any) error
	// Currency resolves a currency code known to the document's Record Type.
	Currency(code string) (types.Currency, bool)
}

// Outcome is the result of one run.
type Outcome struct {
	Status  Status
	Code    string
	Message string
	// Warnings and Logs are what warn and log wrote.
	Warnings []Message
	Logs     []Message
	// Writes are the fields set, in call order.
	Writes []Write
	// Result is the rows a transform returned, in key order.
	Result    []Write
	Steps     uint64
	Reads     int
	Duration  time.Duration
	InputHash string
}

// state is one run in progress.
type state struct {
	host     Host
	limits   Limits
	outcome  *Outcome
	timedOut atomic.Bool
}

func (s *state) message(into *[]Message, code, text string) {
	if len(s.outcome.Warnings)+len(s.outcome.Logs) >= s.limits.MaxMessages {
		return
	}
	*into = append(*into, Message{Code: code, Text: clip(text, maxMessageLen)})
}

func clip(text string, limit int) string {
	if r := []rune(text); len(r) > limit {
		return string(r[:limit]) + "..."
	}
	return text
}

// Run executes the Rule. It never panics and always returns an Outcome.
func (p *Program) Run(ctx context.Context, in Input, host Host, limits Limits) (out Outcome) {
	started := time.Now()
	out = Outcome{Status: OK, InputHash: HashInput(p.Hash, in)}
	s := &state{host: host, limits: limits, outcome: &out}
	thread := &starlark.Thread{Name: in.Rule}
	thread.Print = func(_ *starlark.Thread, text string) { s.message(&out.Logs, "print", text) }
	thread.SetMaxExecutionSteps(limits.MaxSteps)
	timer := time.AfterFunc(limits.Timeout, func() {
		s.timedOut.Store(true)
		thread.Cancel("time limit reached")
	})
	defer timer.Stop()
	stop := context.AfterFunc(ctx, func() { thread.Cancel("the request was cancelled") })
	defer stop()
	defer func() {
		out.Steps = thread.ExecutionSteps()
		out.Duration = time.Since(started)
		if r := recover(); r != nil {
			out.Status, out.Code, out.Message = Errored, CodeError, "the Rule stopped unexpectedly"
		}
	}()

	globals, err := p.prog.Init(thread, s.predeclared(p.Type))
	var result starlark.Value
	if err == nil {
		fn, ok := globals[p.entry].(starlark.Callable)
		if !ok {
			err = fmt.Errorf("%s is not a function", p.entry)
		} else {
			doc, derr := newRecord(in.RecordType, in.Doc)
			if derr != nil {
				err = derr
			} else {
				result, err = starlark.Call(thread, fn, starlark.Tuple{doc, contextValue(in)}, nil)
			}
		}
	}
	if err == nil && p.Type == Transform {
		err = s.collectResult(result)
	}
	if err != nil {
		s.classify(err, thread)
	}
	return out
}

func (s *state) classify(err error, thread *starlark.Thread) {
	out := s.outcome
	var rejection *Rejection
	var hostErr *HostError
	switch {
	case errors.As(err, &rejection):
		out.Status, out.Code, out.Message = Rejected, rejection.Code, rejection.Message
	case s.timedOut.Load():
		out.Status, out.Code, out.Message = Errored, CodeTimeLimit, fmt.Sprintf("the Rule ran longer than %s", s.limits.Timeout)
	case strings.Contains(err.Error(), "too many steps"):
		out.Status, out.Code, out.Message = Errored, CodeStepLimit, fmt.Sprintf("the Rule used more than %d steps", s.limits.MaxSteps)
	case errors.As(err, &hostErr):
		out.Status, out.Code, out.Message = Errored, hostErr.Code, clip(hostErr.Error(), maxMessageLen)
	default:
		message := err.Error()
		var eval *starlark.EvalError
		if errors.As(err, &eval) {
			message = eval.Msg
		}
		var syntaxErr *SourceError
		if errors.As(err, &syntaxErr) {
			message = syntaxErr.Error()
		}
		out.Status, out.Code, out.Message = Errored, CodeError, clip(message, maxMessageLen)
	}
}

// collectResult turns the dict a transform returned into ordered writes.
func (s *state) collectResult(result starlark.Value) error {
	if result == nil || result == starlark.None {
		return nil
	}
	dict, ok := result.(*starlark.Dict)
	if !ok {
		return fmt.Errorf("transform must return a dict, not %s", result.Type())
	}
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return fmt.Errorf("transform result keys must be text")
		}
		value, err := toGo(item[1])
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		s.outcome.Result = append(s.outcome.Result, Write{Field: key, Value: value})
	}
	sort.Slice(s.outcome.Result, func(i, j int) bool { return s.outcome.Result[i].Field < s.outcome.Result[j].Field })
	return nil
}

// contextValue is ctx, the read-only facts of the run.
func contextValue(in Input) starlark.Value {
	now := in.Now.UTC()
	value := starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"now":         starlark.String(now.Format(time.RFC3339)),
		"today":       starlark.String(now.Format(time.DateOnly)),
		"user":        starlark.String(in.User),
		"event":       starlark.String(in.Event),
		"record_type": starlark.String(in.RecordType),
		"request_id":  starlark.String(in.RequestID),
		"rule":        starlark.String(in.Rule),
	})
	value.Freeze()
	return value
}

// HashInput is the SHA-256 of everything a run depends on, so a stored hash
// proves which input produced an outcome without keeping the values.
func HashInput(programHash string, in Input) string {
	h := sha256.New()
	put := func(parts ...string) {
		for _, part := range parts {
			fmt.Fprintf(h, "%d:%s;", len(part), part)
		}
	}
	put(programHash, in.Rule, fmt.Sprint(in.Version), in.Event, in.RecordType, in.RecordID, in.Now.UTC().Format(time.RFC3339Nano), in.User)
	keys := make([]string, 0, len(in.Doc.Values))
	for key := range in.Doc.Values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		put(key, canonicalValue(in.Doc.Values[key]))
	}
	hidden := make([]string, 0, len(in.Doc.Hidden))
	for key, yes := range in.Doc.Hidden {
		if yes {
			hidden = append(hidden, key)
		}
	}
	sort.Strings(hidden)
	put(hidden...)
	return hex.EncodeToString(h.Sum(nil))
}

func canonicalValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		return fmt.Sprintf("bool:%t", v)
	case string:
		return "text:" + v
	case int64:
		return fmt.Sprintf("int:%d", v)
	case types.Decimal:
		return "decimal:" + v.String()
	case types.Money:
		return "money:" + v.String()
	}
	return fmt.Sprintf("%T:%v", value, value)
}

// MaxCascadeDepth is how many Rules may be running inside one another.
const MaxCascadeDepth = 3

// Errors from Stack.Enter.
var (
	ErrCycle = errors.New("a Rule would run itself again")
	ErrDepth = errors.New("Rules are nested too deeply")
)

// Stack is the Rules currently running, outermost first. A Rule cannot start
// another Rule in this phase; the stack is how the kernel will keep that
// bounded once a Rule can cause a write that fires events (spec 13.3).
type Stack []string

// Enter returns the stack with name pushed, or an error when name is already
// running (a cycle) or the depth limit is reached.
func (s Stack) Enter(name string) (Stack, error) {
	for _, running := range s {
		if running == name {
			return s, ErrCycle
		}
	}
	if len(s) >= MaxCascadeDepth {
		return s, ErrDepth
	}
	return append(append(Stack(nil), s...), name), nil
}
