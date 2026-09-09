package reporegistry

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

type Repo struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

func ParseRepo(data []byte) (Repo, error) {
	var r Repo
	if err := yaml.Unmarshal(data, &r); err != nil {
		return Repo{}, fmt.Errorf("parsing repo: %w", err)
	}
	if err := r.Validate(); err != nil {
		return Repo{}, err
	}
	return r, nil
}

func (r Repo) Validate() error {
	if r.Name == "" {
		return errors.New("repo needs a name")
	}
	if r.URL == "" {
		return errors.New("repo needs a url")
	}
	return nil
}
