package reporegistry

import "testing"

func TestParseRepoValid(t *testing.T) {
	data := []byte("name: dx\nurl: https://bitbucket.org/smntlbt/dx.git\n")
	r, err := ParseRepo(data)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "dx" || r.URL != "https://bitbucket.org/smntlbt/dx.git" {
		t.Errorf("r = %+v", r)
	}
}

func TestParseRepoRejectsNoName(t *testing.T) {
	data := []byte("url: https://bitbucket.org/smntlbt/dx.git\n")
	if _, err := ParseRepo(data); err == nil {
		t.Fatal("expected an error for a missing name")
	}
}

func TestParseRepoRejectsNoURL(t *testing.T) {
	data := []byte("name: dx\n")
	if _, err := ParseRepo(data); err == nil {
		t.Fatal("expected an error for a missing url")
	}
}
