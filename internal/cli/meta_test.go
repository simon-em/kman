package cli

import "testing"

func TestMetaRequiresServeSubcommand(t *testing.T) {
	if _, _, code := run("meta"); code == 0 {
		t.Fatal("expected a non-zero exit code with no subcommand")
	}
	if _, _, code := run("meta", "bogus"); code == 0 {
		t.Fatal("expected a non-zero exit code for an unknown subcommand")
	}
}
