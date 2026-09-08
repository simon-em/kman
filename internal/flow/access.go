package flow

import "fmt"

type Access struct {
	Tools       []string `yaml:"tools"`
	Meta        []string `yaml:"meta"`
	Skills      []string `yaml:"skills"`
	Credentials []string `yaml:"credentials"`
	Flows       []string `yaml:"flows"`
}

func (a Access) validate() error {
	for _, list := range [][]string{a.Tools, a.Meta, a.Skills, a.Credentials, a.Flows} {
		for _, v := range list {
			if v == "" {
				return fmt.Errorf("access: an empty entry is not allowed")
			}
		}
	}
	return nil
}

func (a Access) grants(secretName string) bool {
	for _, v := range a.Credentials {
		if v == secretName {
			return true
		}
	}
	return false
}
