// Package rules is the Starlark host of the kernel (spec 13, ADR-0016,
// Proposed). A Rule is a small Starlark program. It runs with no I/O, no
// clock, no randomness and no floats, under step and wall-time limits, and it
// reaches data only through the host functions a Host supplies. The package
// knows nothing about storage or permissions: the kernel's Host does the
// reading as the acting user and validates every write, so a Rule can never
// do more than Go lets it.
//
// Memory is guarded at the host boundary and by a save-time repeat ban (see
// ADR-0020); the interpreter-internal residuals it names stay open.
package rules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"athena-erp/internal/types"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// Type is the kind of a Rule (spec 13.1). Reaction and Scheduled arrive with
// jobs.
type Type string

const (
	Validation Type = "validation"
	Derivation Type = "derivation"
	Guard      Type = "guard"
	Transform  Type = "transform"
	// Posting builds the Posting_Plan a submit writes to the ledgers (batch-2
	// G6, R6.16). A posting Rule defines def post(doc, ctx) returning the plan
	// dict, and def fixtures() returning its replay fixtures. The posting path
	// runs it; the generic engine never does.
	Posting Type = "posting"
)

// Types lists the Rule types in the order they run within one event:
// derivations first, so validations see the derived values, then guards.
// Posting runs on its own path, not in this order, and sits last.
var Types = []Type{Derivation, Validation, Guard, Transform, Posting}

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
	case Posting:
		return "post"
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
	case Posting:
		return []string{"on_submit"}
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
	// MaxValueLen caps the length of one string, list, dict or tuple that
	// crosses between a Rule and the host in either direction.
	MaxValueLen int
	// MaxValues caps the values the host builds for one run: rows read,
	// collections returned, and scalars converted at the boundary. Values
	// the interpreter builds from them are bounded by MaxSteps instead.
	MaxValues int
	// MaxRows caps the rows one host call returns.
	MaxRows int
	// MaxCascadeDepth caps the Rules running inside one another.
	MaxCascadeDepth int
}

// DefaultLimits is what the kernel runs Rules under.
var DefaultLimits = Limits{
	MaxSteps: 200_000, Timeout: 500 * time.Millisecond, MaxReads: 200, MaxWrites: 50, MaxMessages: 50,
	MaxValueLen: 10_000, MaxValues: 50_000, MaxRows: 100, MaxCascadeDepth: MaxCascadeDepth,
}

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
	if t == Posting {
		var fixtures *syntax.DefStmt
		for _, stmt := range file.Stmts {
			if def, ok := stmt.(*syntax.DefStmt); ok && def.Name.Name == "fixtures" {
				fixtures = def
			}
		}
		if fixtures == nil {
			return &SourceError{Line: 1, Msg: "a posting Rule must define def fixtures() returning its replay fixtures"}
		}
		if len(fixtures.Params) != 0 {
			return &SourceError{Line: int(fixtures.Def.Line), Col: int(fixtures.Def.Col), Msg: "fixtures must take no parameters"}
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
			if n.Op == syntax.STAR && (isRepeatable(n.X) || isRepeatable(n.Y)) {
				fail(n.OpPos, "repeating a list, string or tuple with * is not allowed; fetch rows with find or append in a loop")
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

// isRepeatable reports whether an operand is statically a sequence that *
// would repeat: a list, tuple, string or bytes literal, or an allocate call
// that returns a list. Anything else keeps numeric multiplication working;
// the interpreter still refuses a repeat past 1 GiB at run time.
func isRepeatable(n syntax.Expr) bool {
	switch n := n.(type) {
	case *syntax.ParenExpr:
		return isRepeatable(n.X)
	case *syntax.ListExpr, *syntax.TupleExpr:
		return true
	case *syntax.Literal:
		return n.Token == syntax.STRING || n.Token == syntax.BYTES
	case *syntax.CallExpr:
		if id, ok := n.Fn.(*syntax.Ident); ok && id.Name == "allocate" {
			return true
		}
	}
	return false
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
	CodeRejected     = "rejected"
	CodeError        = "error"
	CodeStepLimit    = "step_limit"
	CodeTimeLimit    = "time_limit"
	CodeLimit        = "limit"
	CodeMemoryLimit  = "memory_limit"
	CodeRowLimit     = "row_limit"
	CodeCascadeLimit = "cascade_limit"
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
	Result []Write
	// Plan is what a posting Rule returned. It is nil for every other type
	// and for a run that rejected or errored before collection.
	Plan      *Plan
	Steps     uint64
	Reads     int
	Values    int
	Duration  time.Duration
	InputHash string
}

// Plan is the Posting_Plan a posting Rule's post() returned: one book, one
// posting date and the legs. The kernel names the voucher, event and revision
// around it and validates it through the posting engine, which judges every
// value; this package judges only the shape.
type Plan struct {
	Book        string
	PostingDate string
	Lines       []PlanLine
}

// PlanLine is one leg of a Plan. Exactly one of DebitBase and CreditBase is
// above zero; TxnCurrency with its amounts is the original currency when it
// differs from base; empty means base only.
type PlanLine struct {
	Company     string
	Account     string
	DebitBase   types.Decimal
	CreditBase  types.Decimal
	TxnCurrency string
	TxnDebit    types.Decimal
	TxnCredit   types.Decimal
	Quantity    *types.Decimal
	QuantityUOM string
	Dimensions  map[string]string
	SourceType  string
	SourceID    string
}

// state is one run in progress.
type state struct {
	host     Host
	limits   Limits
	outcome  *Outcome
	timedOut atomic.Bool
	values   int
}

// countValues records n values the host built for the run and stops it past
// the cap.
func (s *state) countValues(n int) error {
	s.values += n
	if s.values > s.limits.MaxValues {
		return &HostError{Code: CodeMemoryLimit, Err: fmt.Errorf("the Rule built more than %d values", s.limits.MaxValues)}
	}
	return nil
}

// checkLen refuses one value longer than the cap.
func (s *state) checkLen(what string, n int) error {
	if n > s.limits.MaxValueLen {
		return &HostError{Code: CodeMemoryLimit, Err: fmt.Errorf("%s is longer than %d", what, s.limits.MaxValueLen)}
	}
	return nil
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
		out.Values = s.values
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
			} else if cerr := s.countValues(len(in.Doc.Values) + 1); cerr != nil {
				err = cerr
			} else {
				result, err = starlark.Call(thread, fn, starlark.Tuple{doc, contextValue(in)}, nil)
			}
		}
	}
	if err == nil && p.Type == Transform {
		err = s.collectResult(result)
	}
	if err == nil && p.Type == Posting {
		err = s.collectPlan(result)
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
	case isRepeatError(err):
		message := err.Error()
		var eval *starlark.EvalError
		if errors.As(err, &eval) {
			message = eval.Msg
		}
		out.Status, out.Code, out.Message = Errored, CodeMemoryLimit, clip(message, maxMessageLen)
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

// isRepeatError reports an interpreter refusal to allocate a repetition. The
// static check catches literal repeats at save time; this catches the same
// bomb through variables at run time, before anything is allocated.
func isRepeatError(err error) bool {
	text := err.Error()
	return strings.Contains(text, "excessive repeat") || strings.Contains(text, "repeat count")
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
	if err := s.checkLen("the transform result", dict.Len()); err != nil {
		return err
	}
	if err := s.countValues(dict.Len()); err != nil {
		return err
	}
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return fmt.Errorf("transform result keys must be text")
		}
		if err := s.checkLen("a transform result key", len(key)); err != nil {
			return err
		}
		value, err := s.toGo(item[1])
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		s.outcome.Result = append(s.outcome.Result, Write{Field: key, Value: value})
	}
	sort.Slice(s.outcome.Result, func(i, j int) bool { return s.outcome.Result[i].Field < s.outcome.Result[j].Field })
	return nil
}

// planKeys are the only keys a plan dict may hold; anything else is a typo
// the author must fix rather than a leg the engine silently drops.
var planKeys = map[string]bool{"book": true, "posting_date": true, "lines": true}

// planLineKeys are the only keys a plan line dict may hold.
var planLineKeys = map[string]bool{
	"company": true, "account": true, "debit": true, "credit": true,
	"txn_currency": true, "txn_debit": true, "txn_credit": true,
	"quantity": true, "quantity_uom": true, "dimensions": true,
	"source_type": true, "source_id": true,
}

// collectPlan turns the dict a posting Rule returned into the Plan the
// posting engine validates. It checks the shape only: presence and scalar
// kinds. Values (balance, open periods, known accounts) are the engine's job,
// so a fixture can replay a plan the current masters would refuse and the
// failure names the value, not the Rule.
func (s *state) collectPlan(result starlark.Value) error {
	if result == nil || result == starlark.None {
		return fmt.Errorf("post must return the plan dict, not nothing")
	}
	dict, ok := result.(*starlark.Dict)
	if !ok {
		return fmt.Errorf("post must return a dict with book, posting_date and lines, not %s", result.Type())
	}
	seen := map[string]starlark.Value{}
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return fmt.Errorf("plan keys must be text")
		}
		if !planKeys[key] {
			return fmt.Errorf("the plan has no key %q; it holds book, posting_date and lines", key)
		}
		seen[key] = item[1]
	}
	for _, key := range []string{"book", "posting_date", "lines"} {
		if _, ok := seen[key]; !ok {
			return fmt.Errorf("the plan needs %q", key)
		}
	}
	book, ok := starlark.AsString(seen["book"])
	if !ok {
		return fmt.Errorf("the plan's book must be text")
	}
	date, ok := starlark.AsString(seen["posting_date"])
	if !ok {
		return fmt.Errorf("the plan's posting_date must be text")
	}
	lines, ok := seen["lines"].(*starlark.List)
	if !ok {
		return fmt.Errorf("the plan's lines must be a list")
	}
	if err := s.checkLen("the plan lines", lines.Len()); err != nil {
		return err
	}
	plan := &Plan{Book: book, PostingDate: date, Lines: make([]PlanLine, 0, lines.Len())}
	for i := range lines.Len() {
		line, err := s.collectPlanLine(i, lines.Index(i))
		if err != nil {
			return err
		}
		plan.Lines = append(plan.Lines, line)
	}
	if err := s.countValues(len(plan.Lines) * 4); err != nil {
		return err
	}
	s.outcome.Plan = plan
	return nil
}

func (s *state) collectPlanLine(index int, value starlark.Value) (PlanLine, error) {
	var line PlanLine
	where := fmt.Sprintf("line %d", index+1)
	dict, ok := value.(*starlark.Dict)
	if !ok {
		return line, fmt.Errorf("%s must be a dict, not %s", where, value.Type())
	}
	seen := map[string]starlark.Value{}
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return line, fmt.Errorf("%s keys must be text", where)
		}
		if !planLineKeys[key] {
			return line, fmt.Errorf("%s has no key %q", where, key)
		}
		seen[key] = item[1]
	}
	text := func(key string) (string, error) {
		raw, ok := seen[key]
		if !ok || raw == starlark.None {
			return "", nil
		}
		text, ok := starlark.AsString(raw)
		if !ok {
			return "", fmt.Errorf("%s %q must be text", where, key)
		}
		return text, nil
	}
	amount := func(key string) (types.Decimal, error) {
		raw, ok := seen[key]
		if !ok || raw == starlark.None {
			return types.Decimal{}, nil
		}
		return s.planAmount(where, key, raw)
	}
	var err error
	if line.Company, err = text("company"); err != nil {
		return line, err
	}
	if line.Account, err = text("account"); err != nil {
		return line, err
	}
	if line.DebitBase, err = amount("debit"); err != nil {
		return line, err
	}
	if line.CreditBase, err = amount("credit"); err != nil {
		return line, err
	}
	if line.TxnCurrency, err = text("txn_currency"); err != nil {
		return line, err
	}
	if line.TxnDebit, err = amount("txn_debit"); err != nil {
		return line, err
	}
	if line.TxnCredit, err = amount("txn_credit"); err != nil {
		return line, err
	}
	if raw, ok := seen["quantity"]; ok && raw != starlark.None {
		quantity, err := s.planAmount(where, "quantity", raw)
		if err != nil {
			return line, err
		}
		line.Quantity = &quantity
	}
	if line.QuantityUOM, err = text("quantity_uom"); err != nil {
		return line, err
	}
	if raw, ok := seen["dimensions"]; ok && raw != starlark.None {
		dimensions, ok := raw.(*starlark.Dict)
		if !ok {
			return line, fmt.Errorf("%s dimensions must be a dict", where)
		}
		if err := s.checkLen(where+" dimensions", dimensions.Len()); err != nil {
			return line, err
		}
		line.Dimensions = map[string]string{}
		for _, item := range dimensions.Items() {
			name, ok := starlark.AsString(item[0])
			if !ok {
				return line, fmt.Errorf("%s dimension names must be text", where)
			}
			member, ok := starlark.AsString(item[1])
			if !ok {
				return line, fmt.Errorf("%s dimension %q must be text", where, name)
			}
			line.Dimensions[name] = member
		}
		if err := s.countValues(len(line.Dimensions)); err != nil {
			return line, err
		}
	}
	if line.SourceType, err = text("source_type"); err != nil {
		return line, err
	}
	if line.SourceID, err = text("source_id"); err != nil {
		return line, err
	}
	return line, nil
}

// planAmount reads one decimal of a plan line: a decimal(), an integer, or
// decimal text. Money is refused on purpose: an amount carries no currency,
// so money(doc.total) would silently drop the code; the author names
// txn_currency beside a decimal instead.
func (s *state) planAmount(where, key string, value starlark.Value) (types.Decimal, error) {
	switch v := value.(type) {
	case decimalValue:
		return v.d, nil
	case starlark.Int:
		if intTooLarge(v) {
			return types.Decimal{}, &HostError{Code: CodeMemoryLimit, Err: fmt.Errorf("%s %q is too large to use as a decimal", where, key)}
		}
		d, err := types.ParseDecimal(v.String())
		if err != nil {
			return types.Decimal{}, fmt.Errorf("%s %q is not a decimal number", where, key)
		}
		return d, nil
	case starlark.String:
		if err := s.checkLen(where+" "+key, len(v)); err != nil {
			return types.Decimal{}, err
		}
		d, err := types.ParseDecimal(string(v))
		if err != nil {
			return types.Decimal{}, fmt.Errorf("%s %q is not a decimal number", where, key)
		}
		return d, nil
	case moneyValue:
		return types.Decimal{}, fmt.Errorf("%s %q needs a decimal, not money: name txn_currency beside it", where, key)
	case starlark.Float:
		return types.Decimal{}, ErrFloat
	}
	return types.Decimal{}, fmt.Errorf("%s %q needs a decimal, an integer or decimal text, not %s", where, key, value.Type())
}

// Fixture is one replay case a posting Rule ships in def fixtures(): a name,
// the header values post() sees as the document, and the child lines staged
// beside it by child Record Type key.
type Fixture struct {
	Name     string
	Values   map[string]any
	Children map[string][]map[string]any
}

// fixtureHost runs fixtures(): the pure builtins work, money resolves through
// the Record Type's currencies, and reads and writes fail because a fixture
// is self-contained synthetic data, never a database read.
type fixtureHost struct {
	currencies func(code string) (types.Currency, bool)
}

func (h fixtureHost) Get(_, _ string) (Row, bool, error) {
	return Row{}, false, &HostError{Code: CodeError, Err: errors.New("fixtures cannot read Records")}
}

func (h fixtureHost) Find(_ string, _ map[string]any, _ int) ([]Row, error) {
	return nil, &HostError{Code: CodeError, Err: errors.New("fixtures cannot read Records")}
}

func (h fixtureHost) Set(_ string, _ any) error {
	return &HostError{Code: CodeError, Err: errors.New("fixtures cannot change fields")}
}

func (h fixtureHost) Currency(code string) (types.Currency, bool) {
	if h.currencies == nil {
		return types.Currency{}, false
	}
	return h.currencies(code)
}

// RunFixtures executes the fixtures() the program defines and returns each
// case. currencies resolves the money codes the fixtures may use. The run
// shares the caller's limits but counts its own steps and values, so fixture
// evaluation cannot silently spend the budget of the replay it feeds.
func (p *Program) RunFixtures(ctx context.Context, limits Limits, currencies func(code string) (types.Currency, bool)) ([]Fixture, error) {
	if p.Type != Posting {
		return nil, fmt.Errorf("only a posting Rule has fixtures")
	}
	s := &state{host: fixtureHost{currencies: currencies}, limits: limits, outcome: &Outcome{}}
	thread := &starlark.Thread{Name: "fixtures"}
	thread.SetMaxExecutionSteps(limits.MaxSteps)
	timer := time.AfterFunc(limits.Timeout, func() {
		s.timedOut.Store(true)
		thread.Cancel("time limit reached")
	})
	defer timer.Stop()
	stop := context.AfterFunc(ctx, func() { thread.Cancel("the request was cancelled") })
	defer stop()
	globals, err := p.prog.Init(thread, s.predeclared(Posting))
	if err != nil {
		return nil, err
	}
	fn, ok := globals["fixtures"].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("fixtures is not a function")
	}
	result, err := starlark.Call(thread, fn, nil, nil)
	if err != nil {
		var rejection *Rejection
		if errors.As(err, &rejection) {
			return nil, rejection
		}
		return nil, purgeFixtureError(err)
	}
	return s.collectFixtures(result)
}

// purgeFixtureError strips the trace from a fixture failure, leaving the
// author's message.
func purgeFixtureError(err error) error {
	var eval *starlark.EvalError
	if errors.As(err, &eval) {
		return errors.New(eval.Msg)
	}
	return err
}

func (s *state) collectFixtures(result starlark.Value) ([]Fixture, error) {
	cases, ok := result.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("fixtures must return a list, not %s", soul(result))
	}
	if cases.Len() == 0 {
		return nil, fmt.Errorf("fixtures must return at least one case")
	}
	if err := s.checkLen("the fixtures", cases.Len()); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]Fixture, 0, cases.Len())
	for i := range cases.Len() {
		where := fmt.Sprintf("fixture %d", i+1)
		dict, ok := cases.Index(i).(*starlark.Dict)
		if !ok {
			return nil, fmt.Errorf("%s must be a dict, not %s", where, soul(cases.Index(i)))
		}
		fields := map[string]starlark.Value{}
		for _, item := range dict.Items() {
			key, ok := starlark.AsString(item[0])
			if !ok {
				return nil, fmt.Errorf("%s keys must be text", where)
			}
			fields[key] = item[1]
		}
		name, ok := fields["name"]
		if !ok {
			return nil, fmt.Errorf("%s needs a name", where)
		}
		text, ok := starlark.AsString(name)
		if !ok || text == "" {
			return nil, fmt.Errorf("%s needs a text name", where)
		}
		if seen[text] {
			return nil, fmt.Errorf("two fixtures are named %q", text)
		}
		seen[text] = true
		fixture := Fixture{Name: text, Values: map[string]any{}, Children: map[string][]map[string]any{}}
		rawValues, ok := fields["values"]
		if !ok {
			return nil, fmt.Errorf("fixture %q needs values", text)
		}
		values, ok := rawValues.(*starlark.Dict)
		if !ok {
			return nil, fmt.Errorf("fixture %q values must be a dict", text)
		}
		converted, err := s.fixtureValues("fixture "+strconv.Quote(text), values)
		if err != nil {
			return nil, err
		}
		fixture.Values = converted
		if rawChildren, ok := fields["children"]; ok && rawChildren != starlark.None {
			children, ok := rawChildren.(*starlark.Dict)
			if !ok {
				return nil, fmt.Errorf("fixture %q children must be a dict of child Record Type to a list of rows", text)
			}
			for _, item := range children.Items() {
				key, ok := starlark.AsString(item[0])
				if !ok {
					return nil, fmt.Errorf("fixture %q children keys must be child Record Type keys", text)
				}
				rows, ok := item[1].(*starlark.List)
				if !ok {
					return nil, fmt.Errorf("fixture %q children %q must be a list of rows", text, key)
				}
				for j := range rows.Len() {
					row, ok := rows.Index(j).(*starlark.Dict)
					if !ok {
						return nil, fmt.Errorf("fixture %q children %q row %d must be a dict", text, key, j+1)
					}
					converted, err := s.fixtureValues(fmt.Sprintf("fixture %q children %q row %d", text, key, j+1), row)
					if err != nil {
						return nil, err
					}
					fixture.Children[key] = append(fixture.Children[key], converted)
				}
			}
		}
		for key := range fields {
			if key != "name" && key != "values" && key != "children" {
				return nil, fmt.Errorf("fixture %q has no key %q; it holds name, values and children", text, key)
			}
		}
		out = append(out, fixture)
	}
	return out, nil
}

// fixtureValues converts one flat dict of field values to Go scalars.
func (s *state) fixtureValues(where string, dict *starlark.Dict) (map[string]any, error) {
	if err := s.checkLen(where, dict.Len()); err != nil {
		return nil, err
	}
	if err := s.countValues(dict.Len()); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("%s keys must be field names", where)
		}
		value, err := s.toGo(item[1])
		if err != nil {
			return nil, fmt.Errorf("%s field %q: %w", where, key, err)
		}
		out[key] = value
	}
	return out, nil
}

// soul names a value's kind for an error without quoting its content.
func soul(value starlark.Value) string {
	if value == nil {
		return "nothing"
	}
	return value.Type()
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
// running (a cycle) or maxDepth is reached.
func (s Stack) Enter(name string, maxDepth int) (Stack, error) {
	for _, running := range s {
		if running == name {
			return s, ErrCycle
		}
	}
	if len(s) >= maxDepth {
		return s, ErrDepth
	}
	return append(append(Stack(nil), s...), name), nil
}
