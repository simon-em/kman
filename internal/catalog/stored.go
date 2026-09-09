package catalog

import (
	"encoding/base64"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

type StoredEntry struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Kind        string `yaml:"kind"`
	Path        string `yaml:"path"`
	Command     string `yaml:"command"`
	Text        string `yaml:"text"`
	Content     string `yaml:"content"`
	SourceURL   string `yaml:"source_url"`
}

func ParseStoredEntry(data []byte) (StoredEntry, error) {
	var e StoredEntry
	if err := yaml.Unmarshal(data, &e); err != nil {
		return StoredEntry{}, fmt.Errorf("parsing skill: %w", err)
	}
	if err := e.Validate(); err != nil {
		return StoredEntry{}, err
	}
	return e, nil
}

func (e StoredEntry) Validate() error {
	if e.Name == "" {
		return errors.New("skill needs a name")
	}
	if e.Kind != KindDoc && e.Kind != KindMCP {
		return fmt.Errorf("kind must be %q or %q", KindDoc, KindMCP)
	}
	if e.Path == "" {
		return errors.New("skill needs a path")
	}
	if e.Text == "" && e.Content == "" {
		return errors.New("skill needs text or content")
	}
	if _, ok := Get(e.Name); ok {
		return fmt.Errorf("%q is already a built-in catalog entry; choose a different name", e.Name)
	}
	return nil
}

func (e StoredEntry) ToEntry() (Entry, error) {
	content, err := e.contentBytes()
	if err != nil {
		return Entry{}, fmt.Errorf("skill %s: %w", e.Name, err)
	}
	entry := Entry{
		Name:        e.Name,
		Description: e.Description,
		Kind:        e.Kind,
		Command:     e.Command,
		Path:        e.Path,
		Content:     content,
	}
	entry.Ref = refFor(content)
	return entry, nil
}

func (e StoredEntry) contentBytes() ([]byte, error) {
	if e.Content != "" {
		return base64.StdEncoding.DecodeString(e.Content)
	}
	return []byte(e.Text), nil
}
