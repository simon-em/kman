package access

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

type User struct {
	ID          string   `yaml:"id"`
	DisplayName string   `yaml:"display_name"`
	SlackUserID string   `yaml:"slack_user_id"`
	Flows       []string `yaml:"flows"`
}

func ParseUser(data []byte) (User, error) {
	var u User
	if err := yaml.Unmarshal(data, &u); err != nil {
		return User{}, fmt.Errorf("parsing user: %w", err)
	}
	if u.ID == "" {
		return User{}, errors.New("user needs an id")
	}
	for _, f := range u.Flows {
		if f == "" {
			return User{}, fmt.Errorf("user %s: an empty entry in flows", u.ID)
		}
	}
	return u, nil
}
