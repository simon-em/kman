package skills

import (
	"testing"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/flow"
)

func bitbucketRef(t *testing.T) string {
	t.Helper()
	e, ok := catalog.Get("bitbucket")
	if !ok {
		t.Fatal("expected a bitbucket catalog entry")
	}
	return "catalog:bitbucket@" + e.Ref
}

func TestComposeAddsCatalogFileAndMCPServer(t *testing.T) {
	spec := flow.Spec{
		Name:   "pr-review",
		Access: flow.Access{Skills: []string{bitbucketRef(t)}},
		Steps:  []flow.Step{{Name: "a", Claude: "review the PR"}},
	}
	out, err := Compose(spec)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("Files = %+v", out.Files)
	}
	srv, ok := out.Steps[0].MCPServers["bitbucket"]
	if !ok {
		t.Fatal("expected a bitbucket mcp server on the claude step")
	}
	if srv.Command != "python3" || len(srv.Args) != 1 || srv.Args[0] != out.Files[0].Path {
		t.Errorf("srv = %+v", srv)
	}
}

func TestComposeRejectsAStaleCatalogRef(t *testing.T) {
	spec := flow.Spec{
		Name:   "pr-review",
		Access: flow.Access{Skills: []string{"catalog:bitbucket@0000deadbeef"}},
		Steps:  []flow.Step{{Name: "a", Claude: "review the PR"}},
	}
	if _, err := Compose(spec); err == nil {
		t.Fatal("expected an error for a stale/wrong pinned ref")
	}
}

func TestComposeRejectsAnUnknownCatalogEntry(t *testing.T) {
	spec := flow.Spec{
		Name:   "pr-review",
		Access: flow.Access{Skills: []string{"catalog:no-such-skill@abc123"}},
		Steps:  []flow.Step{{Name: "a", Claude: "review the PR"}},
	}
	if _, err := Compose(spec); err == nil {
		t.Fatal("expected an error for an unknown catalog entry")
	}
}

func TestComposeWiresALocalSkillByItsOwnPath(t *testing.T) {
	spec := flow.Spec{
		Name: "custom-tool",
		Files: []flow.File{
			{Path: "/kman/tools/my-tool.py", Mode: "0755", Content: "aGVsbG8="},
		},
		Access: flow.Access{Skills: []string{"local:/kman/tools/my-tool.py"}},
		Steps:  []flow.Step{{Name: "a", Claude: "use my tool"}},
	}
	out, err := Compose(spec)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	srv, ok := out.Steps[0].MCPServers["my-tool"]
	if !ok {
		t.Fatal("expected an mcp server named my-tool")
	}
	if srv.Command != "/kman/tools/my-tool.py" || len(srv.Args) != 0 {
		t.Errorf("srv = %+v, want Command to be the file's own path with no args", srv)
	}
	if len(out.Files) != 1 {
		t.Errorf("Files = %+v, local skills should not add a new file", out.Files)
	}
}

func TestComposeWithNoSkillsIsANoOp(t *testing.T) {
	spec := flow.Spec{Name: "plain", Steps: []flow.Step{{Name: "a", Run: "echo hi"}}}
	out, err := Compose(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 0 || len(out.Steps[0].MCPServers) != 0 {
		t.Errorf("out = %+v", out)
	}
}
