package catalog

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"sort"
)

type Entry struct {
	Name        string
	Description string
	Command     string
	Path        string
	Content     []byte
	Ref         string
}

//go:embed assets/bitbucket-mcp.py
var bitbucketScript []byte

var entries = map[string]Entry{}

func init() {
	register(Entry{
		Name:        "bitbucket",
		Description: "Bitbucket pull request tools (list/get PRs, diff, comments) over the REST API.",
		Command:     "python3",
		Path:        "/kman/skills/bitbucket-mcp.py",
		Content:     bitbucketScript,
	})
}

func register(e Entry) {
	e.Ref = refFor(e.Content)
	entries[e.Name] = e
}

func refFor(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])[:12]
}

func Get(name string) (Entry, bool) {
	e, ok := entries[name]
	return e, ok
}

func List() []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
