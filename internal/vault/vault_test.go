package vault

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSetAndGetRoundTrip(t *testing.T) {
	home := t.TempDir()
	v, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set("bitbucket/deploy-key", "s3cr3t"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	value, err := v.Get("bitbucket/deploy-key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if value != "s3cr3t" {
		t.Errorf("Get = %q, want s3cr3t", value)
	}
}

func TestGetMissingReturnsErrNotFound(t *testing.T) {
	home := t.TempDir()
	v, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	_, err = v.Get("no-such-secret")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListReturnsSortedNames(t *testing.T) {
	home := t.TempDir()
	v, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zeta", "alpha", "mid"} {
		if err := v.Set(name, "x"); err != nil {
			t.Fatal(err)
		}
	}
	names, err := v.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "mid", "zeta"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestRemove(t *testing.T) {
	home := t.TempDir()
	v, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set("gone-soon", "x"); err != nil {
		t.Fatal(err)
	}
	if err := v.Remove("gone-soon"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := v.Get("gone-soon"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after Remove, got %v", err)
	}
}

func TestRemoveMissingIsAnError(t *testing.T) {
	home := t.TempDir()
	v, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Remove("never-existed"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestKeyPersistsAcrossOpen(t *testing.T) {
	home := t.TempDir()
	first, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Set("name", "value"); err != nil {
		t.Fatal(err)
	}

	second, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	value, err := second.Get("name")
	if err != nil {
		t.Fatalf("Get with a re-opened vault: %v", err)
	}
	if value != "value" {
		t.Errorf("value = %q, want value", value)
	}
}

func TestSecretsFileIsNotPlaintext(t *testing.T) {
	home := t.TempDir()
	v, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set("name", "a-very-recognizable-secret-value"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, secretsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("expected the secrets file to have content")
	}
	for _, needle := range []string{"a-very-recognizable-secret-value", "name"} {
		if bytes.Contains(data, []byte(needle)) {
			t.Errorf("the secrets file on disk contains %q in the clear", needle)
		}
	}
}

func TestKeyFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix file permissions don't apply")
	}
	home := t.TempDir()
	if _, err := Open(home); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(home, keyFileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("vault.key mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestDecryptFailsWithAWrongKey(t *testing.T) {
	home := t.TempDir()
	v, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set("name", "value"); err != nil {
		t.Fatal(err)
	}

	wrongKey := make([]byte, 32)
	if err := os.WriteFile(filepath.Join(home, keyFileName), wrongKey, 0o600); err != nil {
		t.Fatal(err)
	}

	tampered, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tampered.Get("name"); err == nil {
		t.Fatal("expected an error decrypting with the wrong key")
	}
}
