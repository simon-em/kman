package flow

import "testing"

func TestParseSkillRefCatalog(t *testing.T) {
	ref, err := ParseSkillRef("catalog:bitbucket@abc123")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Kind != "catalog" || ref.Name != "bitbucket" || ref.Ref != "abc123" {
		t.Errorf("ref = %+v", ref)
	}
}

func TestParseSkillRefLocal(t *testing.T) {
	ref, err := ParseSkillRef("local:/kman/tools/my-tool.py")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Kind != "local" || ref.Path != "/kman/tools/my-tool.py" || ref.Name != "my-tool" {
		t.Errorf("ref = %+v", ref)
	}
}

func TestParseSkillRefRejectsCatalogWithNoRef(t *testing.T) {
	if _, err := ParseSkillRef("catalog:bitbucket"); err == nil {
		t.Fatal("expected an error for a catalog ref with no @ref")
	}
}

func TestParseSkillRefRejectsAnUnknownPrefix(t *testing.T) {
	if _, err := ParseSkillRef("bitbucket"); err == nil {
		t.Fatal("expected an error for a ref with no catalog:/local: prefix")
	}
}

func TestParseSkillRefRejectsAnEmptyLocalPath(t *testing.T) {
	if _, err := ParseSkillRef("local:"); err == nil {
		t.Fatal("expected an error for an empty local path")
	}
}
