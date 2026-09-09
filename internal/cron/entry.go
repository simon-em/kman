package cron

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

const CreateMeta = "cron.create"

type Entry struct {
	Name      string            `yaml:"name"`
	Flow      string            `yaml:"flow"`
	Schedule  string            `yaml:"schedule"`
	Args      map[string]string `yaml:"args"`
	CreatedBy string            `yaml:"created_by"`
}

func ParseEntry(data []byte) (Entry, error) {
	var e Entry
	if err := yaml.Unmarshal(data, &e); err != nil {
		return Entry{}, fmt.Errorf("parsing cron entry: %w", err)
	}
	if err := e.Validate(); err != nil {
		return Entry{}, err
	}
	return e, nil
}

func (e Entry) Validate() error {
	if e.Name == "" {
		return errors.New("cron entry needs a name")
	}
	if e.Flow == "" {
		return errors.New("cron entry needs a flow")
	}
	if e.Schedule == "" {
		return errors.New("cron entry needs a schedule")
	}
	if _, err := ParseSchedule(e.Schedule); err != nil {
		return fmt.Errorf("cron entry %s: %w", e.Name, err)
	}
	return nil
}
