package access

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

type Group struct {
	Name    string   `yaml:"name"`
	Members []string `yaml:"members"`
	Flows   []string `yaml:"flows"`
}

func ParseGroup(data []byte) (Group, error) {
	var g Group
	if err := yaml.Unmarshal(data, &g); err != nil {
		return Group{}, fmt.Errorf("parsing group: %w", err)
	}
	if g.Name == "" {
		return Group{}, errors.New("group needs a name")
	}
	for _, m := range g.Members {
		if m == "" {
			return Group{}, fmt.Errorf("group %s: an empty entry in members", g.Name)
		}
	}
	for _, f := range g.Flows {
		if f == "" {
			return Group{}, fmt.Errorf("group %s: an empty entry in flows", g.Name)
		}
	}
	return g, nil
}
