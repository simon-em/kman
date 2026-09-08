package access

import "fmt"

type Registry struct {
	Users  map[string]User
	Groups map[string]Group
}

func NewRegistry() Registry {
	return Registry{Users: map[string]User{}, Groups: map[string]Group{}}
}

func (r Registry) UserBySlackID(slackID string) (User, bool) {
	if slackID == "" {
		return User{}, false
	}
	for _, u := range r.Users {
		if u.SlackUserID == slackID {
			return u, true
		}
	}
	return User{}, false
}

func (r Registry) CanTrigger(userID, flowName string) bool {
	if u, ok := r.Users[userID]; ok && contains(u.Flows, flowName) {
		return true
	}
	for _, g := range r.Groups {
		if contains(g.Members, userID) && contains(g.Flows, flowName) {
			return true
		}
	}
	return false
}

func (r Registry) Validate(knownFlows map[string]bool) error {
	for id, u := range r.Users {
		for _, f := range u.Flows {
			if !knownFlows[f] {
				return fmt.Errorf("user %s: grants unknown flow %q", id, f)
			}
		}
	}
	for name, g := range r.Groups {
		for _, m := range g.Members {
			if _, ok := r.Users[m]; !ok {
				return fmt.Errorf("group %s: member %q is not a known user", name, m)
			}
		}
		for _, f := range g.Flows {
			if !knownFlows[f] {
				return fmt.Errorf("group %s: grants unknown flow %q", name, f)
			}
		}
	}
	return nil
}

func contains(list []string, needle string) bool {
	for _, v := range list {
		if v == needle {
			return true
		}
	}
	return false
}
