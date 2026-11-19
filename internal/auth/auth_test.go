package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	const password = "correct horse battery"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$") || strings.Contains(hash, password) {
		t.Fatalf("hash = %q", hash)
	}
	if ok, rehash := VerifyPassword(password, hash); !ok || rehash {
		t.Fatalf("verify = %v, rehash = %v", ok, rehash)
	}
	if ok, _ := VerifyPassword(password+"x", hash); ok {
		t.Fatal("a wrong password matched")
	}
	other, _ := HashPassword(password)
	if other == hash {
		t.Fatal("two hashes of one password share a salt")
	}
}

func TestWeakerHashMatchesAndAsksForRehash(t *testing.T) {
	hash, err := hashWith("correct horse battery", Params{MemoryKiB: 8 * 1024, Time: 1, Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ok, rehash := VerifyPassword("correct horse battery", hash); !ok || !rehash {
		t.Fatalf("verify = %v, rehash = %v", ok, rehash)
	}
}

func TestMalformedOrHostileHashNeverMatches(t *testing.T) {
	for _, hash := range []string{"", "plain", "$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA", "$argon2id$v=19$m=999999999,t=3,p=4$c2FsdA$aGFzaA", "$argon2id$v=19$m=65536,t=3,p=4$$"} {
		if ok, _ := VerifyPassword("x", hash); ok {
			t.Fatalf("%q matched", hash)
		}
	}
}

func TestPasswordPolicy(t *testing.T) {
	if CheckPassword("alice", "short") == nil {
		t.Fatal("a short password was accepted")
	}
	if CheckPassword("alice-long-name", "ALICE-LONG-NAME") == nil {
		t.Fatal("the username was accepted as password")
	}
	if err := CheckPassword("alice", "a long enough passphrase"); err != nil {
		t.Fatal(err)
	}
}

func grant(recordType string, level int, condition string, actions ...string) Grant {
	g := Grant{RecordType: recordType, Level: level, Condition: condition, Actions: map[string]bool{}}
	for _, a := range actions {
		g.Actions[a] = true
	}
	return g
}

func TestNoGrantMeansDenyAndGrantsAdd(t *testing.T) {
	p := &Principal{}
	for _, action := range Actions {
		if p.Can("invoice", action) {
			t.Fatalf("%s allowed with no Grant", action)
		}
	}
	if conditions, all := p.Scope("invoice", ActionRead); all || len(conditions) != 0 {
		t.Fatal("no Grant gave a read scope")
	}
	p.Grants = []Grant{grant("invoice", 1, "doc.owner == user.name", ActionRead), grant("invoice", 3, "", ActionCreate)}
	if !p.Can("invoice", ActionRead) || !p.Can("invoice", ActionCreate) || p.Can("invoice", ActionDelete) || p.Can("order", ActionRead) {
		t.Fatal("Grants are not additive per action and Record Type")
	}
	if p.Level("invoice") != 3 || p.Level("order") != 0 {
		t.Fatalf("level = %d", p.Level("invoice"))
	}
	if conditions, all := p.Scope("invoice", ActionRead); all || len(conditions) != 1 {
		t.Fatalf("read scope = %v, %v", conditions, all)
	}
	p.Grants = append(p.Grants, grant(AllRecordTypes, 0, "", ActionRead))
	if _, all := p.Scope("invoice", ActionRead); !all {
		t.Fatal("an unconditional Grant did not lift the row condition")
	}
	if !(&Principal{Owner: true}).Can("anything", ActionDelete) {
		t.Fatal("the owner was denied")
	}
}

func TestSeparateDutiesNeedsEveryGrant(t *testing.T) {
	g := grant("invoice", 0, "", ActionSubmit)
	g.SeparateDuties = true
	p := &Principal{Grants: []Grant{g}}
	if !p.MustSeparateDuties("invoice", ActionSubmit) {
		t.Fatal("separate duties not applied")
	}
	p.Grants = append(p.Grants, grant("invoice", 0, "", ActionSubmit))
	if p.MustSeparateDuties("invoice", ActionSubmit) {
		t.Fatal("a Grant without separate duties should allow the creator")
	}
}

func TestCSRFTokenIsBoundToTheSession(t *testing.T) {
	token := CSRFToken("session-a")
	if !CheckCSRF("session-a", token) || CheckCSRF("session-b", token) || CheckCSRF("session-a", "") {
		t.Fatal("CSRF check is not bound to the session")
	}
}

func TestValidateRole(t *testing.T) {
	bad := []Role{
		{Name: ""},
		{Name: "R", SystemPermissions: []string{"root"}},
		{Name: "R", Grants: []Grant{grant("invoice", 0, "")}},
		{Name: "R", Grants: []Grant{grant("invoice", 10, "", ActionRead)}},
		{Name: "R", Grants: []Grant{grant("invoice", 0, "", "fly")}},
		{Name: "R", Grants: []Grant{grant(AllRecordTypes, 0, "doc.x == 1", ActionRead)}},
	}
	for _, r := range bad {
		if ValidateRole(r) == nil {
			t.Fatalf("role %+v was accepted", r)
		}
	}
	if err := ValidateRole(Role{Name: "Clerk", SystemPermissions: []string{PermissionBuild}, Grants: []Grant{grant("invoice", 2, "", ActionRead)}}); err != nil {
		t.Fatal(err)
	}
}
