package catalog

import "testing"

func TestParseStoredEntryValid(t *testing.T) {
	data := []byte("name: my-skill\ndescription: does a thing\nkind: doc\npath: .claude/skills/my-skill/SKILL.md\ntext: hello\n")
	e, err := ParseStoredEntry(data)
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "my-skill" || e.Kind != KindDoc || e.Text != "hello" {
		t.Errorf("e = %+v", e)
	}
}

func TestParseStoredEntryRejectsNoName(t *testing.T) {
	data := []byte("kind: doc\npath: x\ntext: hi\n")
	if _, err := ParseStoredEntry(data); err == nil {
		t.Fatal("expected an error for a missing name")
	}
}

func TestParseStoredEntryRejectsABadKind(t *testing.T) {
	data := []byte("name: x\nkind: nonsense\npath: x\ntext: hi\n")
	if _, err := ParseStoredEntry(data); err == nil {
		t.Fatal("expected an error for an invalid kind")
	}
}

func TestParseStoredEntryRejectsNoPath(t *testing.T) {
	data := []byte("name: x\nkind: doc\ntext: hi\n")
	if _, err := ParseStoredEntry(data); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}

func TestParseStoredEntryRejectsNoContent(t *testing.T) {
	data := []byte("name: x\nkind: doc\npath: x\n")
	if _, err := ParseStoredEntry(data); err == nil {
		t.Fatal("expected an error when neither text nor content is set")
	}
}

func TestParseStoredEntryRejectsABuiltinNameCollision(t *testing.T) {
	data := []byte("name: bitbucket\nkind: doc\npath: x\ntext: hi\n")
	if _, err := ParseStoredEntry(data); err == nil {
		t.Fatal("expected an error for a name colliding with a built-in entry")
	}
}

func TestStoredEntryToEntryFromText(t *testing.T) {
	e := StoredEntry{Name: "my-skill", Kind: KindDoc, Path: "x", Text: "hello"}
	entry, err := e.ToEntry()
	if err != nil {
		t.Fatal(err)
	}
	if string(entry.Content) != "hello" {
		t.Errorf("Content = %q", entry.Content)
	}
	if entry.Ref != refFor([]byte("hello")) {
		t.Errorf("Ref = %q", entry.Ref)
	}
}

func TestStoredEntryToEntryFromBase64Content(t *testing.T) {
	e := StoredEntry{Name: "my-skill", Kind: KindMCP, Path: "x", Content: "aGVsbG8="}
	entry, err := e.ToEntry()
	if err != nil {
		t.Fatal(err)
	}
	if string(entry.Content) != "hello" {
		t.Errorf("Content = %q", entry.Content)
	}
}

func TestStoredEntryToEntryRejectsInvalidBase64(t *testing.T) {
	e := StoredEntry{Name: "my-skill", Kind: KindDoc, Path: "x", Content: "not base64!"}
	if _, err := e.ToEntry(); err == nil {
		t.Fatal("expected an error for invalid base64 content")
	}
}
