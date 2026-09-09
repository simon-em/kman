package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/simon-em/kman/internal/access"
	"github.com/simon-em/kman/internal/cron"
	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/reporegistry"
	"gopkg.in/yaml.v3"
)

func LoadFlow(home, name string) (flow.Spec, error) {
	data, err := ReadFlowRaw(home, name)
	if err != nil {
		return flow.Spec{}, err
	}
	return flow.Parse(data)
}

func ReadFlowRaw(home, name string) ([]byte, error) {
	return os.ReadFile(flowPath(home, name))
}

func LoadUser(home, id string) (access.User, error) {
	data, err := os.ReadFile(userPath(home, id))
	if err != nil {
		return access.User{}, err
	}
	return access.ParseUser(data)
}

func LoadGroup(home, name string) (access.Group, error) {
	data, err := os.ReadFile(groupPath(home, name))
	if err != nil {
		return access.Group{}, err
	}
	return access.ParseGroup(data)
}

func LoadCronEntry(home, name string) (cron.Entry, error) {
	data, err := os.ReadFile(cronPath(home, name))
	if err != nil {
		return cron.Entry{}, err
	}
	return cron.ParseEntry(data)
}

func LoadRepo(home, name string) (reporegistry.Repo, error) {
	data, err := os.ReadFile(repoPath(home, name))
	if err != nil {
		return reporegistry.Repo{}, err
	}
	return reporegistry.ParseRepo(data)
}

func ListRepos(home string) ([]reporegistry.Repo, error) {
	names, err := ListRepoNames(home)
	if err != nil {
		return nil, err
	}
	repos := make([]reporegistry.Repo, 0, len(names))
	for _, name := range names {
		r, err := LoadRepo(home, name)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, nil
}

func SaveFlow(home string, spec flow.Spec, author string) error {
	if err := writeYAML(flowPath(home, spec.Name), spec); err != nil {
		return err
	}
	return commit(Dir(home), fmt.Sprintf("flow %s: saved", spec.Name), author)
}

func SaveUser(home string, u access.User, author string) error {
	if err := writeYAML(userPath(home, u.ID), u); err != nil {
		return err
	}
	return commit(Dir(home), fmt.Sprintf("user %s: saved", u.ID), author)
}

func SaveGroup(home string, g access.Group, author string) error {
	if err := writeYAML(groupPath(home, g.Name), g); err != nil {
		return err
	}
	return commit(Dir(home), fmt.Sprintf("group %s: saved", g.Name), author)
}

func SaveCronEntry(home string, e cron.Entry, author string) error {
	if err := writeYAML(cronPath(home, e.Name), e); err != nil {
		return err
	}
	return commit(Dir(home), fmt.Sprintf("cron %s: saved", e.Name), author)
}

func RemoveCronEntry(home, name, author string) error {
	if err := os.Remove(cronPath(home, name)); err != nil {
		return err
	}
	return commit(Dir(home), fmt.Sprintf("cron %s: removed", name), author)
}

func SaveRepo(home string, r reporegistry.Repo, author string) error {
	if err := writeYAML(repoPath(home, r.Name), r); err != nil {
		return err
	}
	return commit(Dir(home), fmt.Sprintf("repo %s: saved", r.Name), author)
}

func RemoveRepo(home, name, author string) error {
	if err := os.Remove(repoPath(home, name)); err != nil {
		return err
	}
	return commit(Dir(home), fmt.Sprintf("repo %s: removed", name), author)
}

func ListFlowNames(home string) ([]string, error) {
	return listYAMLBasenames(filepath.Join(Dir(home), "flows"))
}

func ListUserIDs(home string) ([]string, error) {
	return listYAMLBasenames(filepath.Join(Dir(home), "users"))
}

func ListGroupNames(home string) ([]string, error) {
	return listYAMLBasenames(filepath.Join(Dir(home), "groups"))
}

func ListCronNames(home string) ([]string, error) {
	return listYAMLBasenames(filepath.Join(Dir(home), "cron"))
}

func ListRepoNames(home string) ([]string, error) {
	return listYAMLBasenames(filepath.Join(Dir(home), "repos"))
}

func flowPath(home, name string) string {
	return filepath.Join(Dir(home), "flows", name+".yaml")
}

func userPath(home, id string) string {
	return filepath.Join(Dir(home), "users", id+".yaml")
}

func groupPath(home, name string) string {
	return filepath.Join(Dir(home), "groups", name+".yaml")
}

func cronPath(home, name string) string {
	return filepath.Join(Dir(home), "cron", name+".yaml")
}

func repoPath(home, name string) string {
	return filepath.Join(Dir(home), "repos", name+".yaml")
}

func listYAMLBasenames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(names)
	return names, nil
}

func writeYAML(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func commit(dir, message, author string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return nil
	}
	if err := runGit(dir, "add", "-A"); err != nil {
		return err
	}
	authorName := author
	if authorName == "" {
		authorName = "kman"
	}
	cmd := gitCommitCmd(dir, message, authorName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit: %w: %s", err, out)
	}
	return nil
}
