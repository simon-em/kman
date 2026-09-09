package catalog

import "testing"

func TestGetBitbucket(t *testing.T) {
	e, ok := Get("bitbucket")
	if !ok {
		t.Fatal("expected a bitbucket entry")
	}
	if e.Kind != KindDoc || e.Path == "" || len(e.Content) == 0 {
		t.Errorf("e = %+v", e)
	}
	if e.Ref == "" {
		t.Error("expected a non-empty pinned ref")
	}
}

func TestGetUnknownEntry(t *testing.T) {
	if _, ok := Get("no-such-skill"); ok {
		t.Fatal("expected ok=false for an unknown entry")
	}
}

func TestRefIsStableAndContentDerived(t *testing.T) {
	a, _ := Get("bitbucket")
	b, _ := Get("bitbucket")
	if a.Ref != b.Ref {
		t.Errorf("Ref is not stable across calls: %q != %q", a.Ref, b.Ref)
	}
	if a.Ref != refFor(a.Content) {
		t.Errorf("Ref = %q, want it derived from the entry's own content", a.Ref)
	}
}

func TestListIsSortedByName(t *testing.T) {
	list := List()
	if len(list) == 0 {
		t.Fatal("expected at least one catalog entry")
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].Name > list[i].Name {
			t.Errorf("List() not sorted: %q before %q", list[i-1].Name, list[i].Name)
		}
	}
}
