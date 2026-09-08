package access

import "testing"

func TestParseGroupValid(t *testing.T) {
	g, err := ParseGroup([]byte("name: oncall\nmembers: [simon, alex]\nflows: [deploy-review, run-tests]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "oncall" || len(g.Members) != 2 || len(g.Flows) != 2 {
		t.Errorf("g = %+v", g)
	}
}

func TestParseGroupRejectsMissingName(t *testing.T) {
	if _, err := ParseGroup([]byte("members: [simon]\n")); err == nil {
		t.Fatal("expected an error for a group with no name")
	}
}

func TestParseGroupRejectsEmptyMemberEntry(t *testing.T) {
	if _, err := ParseGroup([]byte("name: oncall\nmembers: [\"\"]\n")); err == nil {
		t.Fatal("expected an error for an empty members entry")
	}
}
