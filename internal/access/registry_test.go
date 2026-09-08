package access

import "testing"

func TestUserBySlackID(t *testing.T) {
	r := NewRegistry()
	r.Users["simon"] = User{ID: "simon", SlackUserID: "U123"}

	u, ok := r.UserBySlackID("U123")
	if !ok || u.ID != "simon" {
		t.Errorf("UserBySlackID = %+v, %v", u, ok)
	}

	if _, ok := r.UserBySlackID("U999"); ok {
		t.Error("expected no match for an unknown Slack ID")
	}
}

func TestCanTriggerViaDirectUserGrant(t *testing.T) {
	r := NewRegistry()
	r.Users["simon"] = User{ID: "simon", Flows: []string{"deploy-review"}}

	if !r.CanTrigger("simon", "deploy-review") {
		t.Error("expected simon to be able to trigger deploy-review")
	}
	if r.CanTrigger("simon", "run-tests") {
		t.Error("simon should not be able to trigger run-tests")
	}
}

func TestCanTriggerViaGroupMembership(t *testing.T) {
	r := NewRegistry()
	r.Users["alex"] = User{ID: "alex"}
	r.Groups["oncall"] = Group{Name: "oncall", Members: []string{"alex"}, Flows: []string{"run-tests"}}

	if !r.CanTrigger("alex", "run-tests") {
		t.Error("expected alex to trigger run-tests via the oncall group")
	}
	if r.CanTrigger("someone-else", "run-tests") {
		t.Error("a non-member should not inherit the group's grant")
	}
}

func TestZeroAccessByDefault(t *testing.T) {
	r := NewRegistry()
	r.Users["simon"] = User{ID: "simon"}
	if r.CanTrigger("simon", "anything") {
		t.Error("a user with no grants and no group membership must not be able to trigger anything")
	}
}

func TestValidateCatchesUnknownFlowOnUser(t *testing.T) {
	r := NewRegistry()
	r.Users["simon"] = User{ID: "simon", Flows: []string{"no-such-flow"}}
	if err := r.Validate(map[string]bool{"deploy-review": true}); err == nil {
		t.Fatal("expected an error for a grant referencing an unknown flow")
	}
}

func TestValidateCatchesUnknownFlowOnGroup(t *testing.T) {
	r := NewRegistry()
	r.Users["simon"] = User{ID: "simon"}
	r.Groups["oncall"] = Group{Name: "oncall", Members: []string{"simon"}, Flows: []string{"no-such-flow"}}
	if err := r.Validate(map[string]bool{"deploy-review": true}); err == nil {
		t.Fatal("expected an error for a group grant referencing an unknown flow")
	}
}

func TestValidateCatchesUnknownGroupMember(t *testing.T) {
	r := NewRegistry()
	r.Groups["oncall"] = Group{Name: "oncall", Members: []string{"nobody"}}
	if err := r.Validate(map[string]bool{}); err == nil {
		t.Fatal("expected an error for a group member that is not a known user")
	}
}

func TestValidatePasses(t *testing.T) {
	r := NewRegistry()
	r.Users["simon"] = User{ID: "simon", Flows: []string{"deploy-review"}}
	r.Groups["oncall"] = Group{Name: "oncall", Members: []string{"simon"}, Flows: []string{"run-tests"}}
	if err := r.Validate(map[string]bool{"deploy-review": true, "run-tests": true}); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
