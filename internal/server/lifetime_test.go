package server

import (
	"context"
	"reflect"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/jobs"
)

func TestValidateLifetime(t *testing.T) {
	if err := validateLifetime(Lifetime{}); err != nil {
		t.Fatalf("empty lifetime: %v", err)
	}
	full := Lifetime{Profile: "guest", RetentionTrigger: jobs.TriggerCreated, RetentionClass: jobs.ClassGuest, DisposalAction: jobs.ActionAnonymise, ArchiveReaders: []string{"Clerk"}, Hold: true}
	if err := validateLifetime(full); err != nil {
		t.Fatalf("full lifetime: %v", err)
	}
	cases := []struct {
		name string
		mute func(*Lifetime)
	}{
		{"profile too long", func(l *Lifetime) { l.Profile = strings.Repeat("p", 41) }},
		{"missing class", func(l *Lifetime) { l.RetentionClass = "" }},
		{"bad class", func(l *Lifetime) { l.RetentionClass = "forever" }},
		{"bad trigger", func(l *Lifetime) { l.RetentionTrigger = "someday" }},
		{"bad action", func(l *Lifetime) { l.DisposalAction = "shred" }},
		{"empty reader", func(l *Lifetime) { l.ArchiveReaders = []string{""} }},
		{"long reader", func(l *Lifetime) { l.ArchiveReaders = []string{strings.Repeat("r", 141)} }},
	}
	for _, tc := range cases {
		broken := full
		broken.ArchiveReaders = append([]string(nil), full.ArchiveReaders...)
		tc.mute(&broken)
		if err := validateLifetime(broken); err == nil {
			t.Errorf("%s: validation passed, want an error", tc.name)
		}
	}
	triggerless := full
	triggerless.RetentionTrigger = ""
	if err := validateLifetime(triggerless); err != nil {
		t.Errorf("empty trigger: %v, want the created default", err)
	}
	actionless := full
	actionless.DisposalAction = ""
	if err := validateLifetime(actionless); err != nil {
		t.Errorf("empty action: %v, want the class default", err)
	}
}

func TestCarryLifetime(t *testing.T) {
	previous := &recordTypeDefinition{}
	previous.Lifetime = Lifetime{Profile: "guest", RetentionClass: jobs.ClassGuest}
	next := recordTypeDefinition{Name: "guest"}
	carryLifetime(&next, previous)
	if !reflect.DeepEqual(next.Lifetime, previous.Lifetime) {
		t.Fatalf("carried %+v, want %+v", next.Lifetime, previous.Lifetime)
	}
	fresh := recordTypeDefinition{Name: "guest"}
	carryLifetime(&fresh, nil)
	if !reflect.DeepEqual(fresh.Lifetime, Lifetime{}) {
		t.Fatalf("create carried %+v, want unset", fresh.Lifetime)
	}
}

func TestSavePreservesLifetime(t *testing.T) {
	ctx := context.Background()
	recordTypes, _ := newServices(t)
	fields := []*athenav1.RecordTypeField{{Key: "name", Label: "Name", Type: "Text", Required: true}}
	saveRecordType(t, recordTypes, "guest", fields...)
	// A future build stores a lifetime block the proto cannot express
	// yet; inject one the way such a build would leave it.
	if _, err := recordTypes.db.ExecContext(ctx, `UPDATE athena_record_types SET definition = JSON_SET(definition, '$.lifetime', 'guest', '$.retention_class', 'guest', '$.retention_trigger', 'created', '$.disposal_action', 'anonymise') WHERE name = 'guest'`); err != nil {
		t.Fatalf("inject lifetime: %v", err)
	}
	res, err := recordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "guest", Label: "Guest", Fields: fields}, ExpectedRevision: 1})
	if err != nil {
		t.Fatalf("resave: %v", err)
	}
	if res.GetRecordType().GetRevision() != 2 {
		t.Fatalf("revision = %d, want 2", res.GetRecordType().GetRevision())
	}
	loaded, _, err := loadRecordType(ctx, recordTypes.db, "guest")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := Lifetime{Profile: "guest", RetentionTrigger: jobs.TriggerCreated, RetentionClass: jobs.ClassGuest, DisposalAction: jobs.ActionAnonymise}
	if loaded.Lifetime.Profile != want.Profile || loaded.Lifetime.RetentionTrigger != want.RetentionTrigger || loaded.Lifetime.RetentionClass != want.RetentionClass || loaded.Lifetime.DisposalAction != want.DisposalAction {
		t.Fatalf("lifetime after save is %+v, want %+v", loaded.Lifetime, want)
	}
}
