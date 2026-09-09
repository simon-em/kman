package web

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/simon-em/kman/internal/flow"
)

const blankRows = 3
const blankSteps = 2

type argRow struct {
	Name, Default, Description string
	Required                   bool
}

type kvRow struct{ Name, Value string }

type fileRow struct{ Path, Mode, Text string }

type stepRow struct {
	Name            string
	Type            string
	Body            string
	AllowedTools    string
	DisallowedTools string
	MaxTurns        string
	Effort          string
	Model           string
	PermissionMode  string
	ContinueOnError bool
}

func withSpares[T any](rows []T, n int) []T {
	return append(rows, make([]T, n)...)
}

func mapToRows(m map[string]string) []kvRow {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	rows := make([]kvRow, 0, len(names))
	for _, n := range names {
		rows = append(rows, kvRow{Name: n, Value: m[n]})
	}
	return rows
}

func formCount(r *http.Request, prefix string) int {
	n, _ := strconv.Atoi(r.FormValue(prefix + "_count"))
	return n
}

func parseArgs(r *http.Request) map[string]flow.Arg {
	out := map[string]flow.Arg{}
	for i := 0; i < formCount(r, "arg"); i++ {
		name := strings.TrimSpace(r.FormValue(fmt.Sprintf("arg_name_%d", i)))
		if name == "" {
			continue
		}
		out[name] = flow.Arg{
			Default:     r.FormValue(fmt.Sprintf("arg_default_%d", i)),
			Description: r.FormValue(fmt.Sprintf("arg_description_%d", i)),
			Required:    r.FormValue(fmt.Sprintf("arg_required_%d", i)) == "on",
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseKV(r *http.Request, prefix string) map[string]string {
	out := map[string]string{}
	for i := 0; i < formCount(r, prefix); i++ {
		name := strings.TrimSpace(r.FormValue(fmt.Sprintf("%s_name_%d", prefix, i)))
		if name == "" {
			continue
		}
		out[name] = r.FormValue(fmt.Sprintf("%s_value_%d", prefix, i))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseList(r *http.Request, prefix string) []string {
	var out []string
	for i := 0; i < formCount(r, prefix); i++ {
		v := strings.TrimSpace(r.FormValue(fmt.Sprintf("%s_%d", prefix, i)))
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func splitFields(s string) []string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func parseFiles(r *http.Request) []flow.File {
	var out []flow.File
	for i := 0; i < formCount(r, "file"); i++ {
		path := strings.TrimSpace(r.FormValue(fmt.Sprintf("file_path_%d", i)))
		if path == "" {
			continue
		}
		out = append(out, flow.File{
			Path: path,
			Mode: r.FormValue(fmt.Sprintf("file_mode_%d", i)),
			Text: r.FormValue(fmt.Sprintf("file_text_%d", i)),
		})
	}
	return out
}

func parseSteps(r *http.Request) []flow.Step {
	var out []flow.Step
	for i := 0; i < formCount(r, "step"); i++ {
		name := strings.TrimSpace(r.FormValue(fmt.Sprintf("step_name_%d", i)))
		body := r.FormValue(fmt.Sprintf("step_body_%d", i))
		if name == "" && strings.TrimSpace(body) == "" {
			continue
		}
		step := flow.Step{
			Name:            name,
			AllowedTools:    splitFields(r.FormValue(fmt.Sprintf("step_allowed_%d", i))),
			DisallowedTools: splitFields(r.FormValue(fmt.Sprintf("step_disallowed_%d", i))),
			Model:           r.FormValue(fmt.Sprintf("step_model_%d", i)),
			Effort:          r.FormValue(fmt.Sprintf("step_effort_%d", i)),
			PermissionMode:  r.FormValue(fmt.Sprintf("step_permission_mode_%d", i)),
			ContinueOn:      r.FormValue(fmt.Sprintf("step_continue_%d", i)) == "on",
		}
		if v := r.FormValue(fmt.Sprintf("step_max_turns_%d", i)); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				step.MaxTurns = n
			}
		}
		if r.FormValue(fmt.Sprintf("step_type_%d", i)) == "claude" {
			step.Claude = body
		} else {
			step.Run = body
		}
		out = append(out, step)
	}
	return out
}

func parseMeta(r *http.Request) []string {
	var out []string
	if r.FormValue("meta_ask") == "on" {
		out = append(out, "ask")
	}
	if r.FormValue("meta_cron_create") == "on" {
		out = append(out, "cron.create")
	}
	return out
}

func formToSpec(r *http.Request) flow.Spec {
	cpus := 0
	if v := r.FormValue("cpus"); v != "" {
		cpus, _ = strconv.Atoi(v)
	}
	return flow.Spec{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: r.FormValue("description"),
		Repo:        strings.TrimSpace(r.FormValue("repo")),
		Branch:      r.FormValue("branch"),
		Kranqfile:   r.FormValue("kranqfile"),
		Resources: flow.Resources{
			Memory: r.FormValue("memory"),
			CPUs:   cpus,
			Disk:   r.FormValue("disk"),
		},
		Args:        parseArgs(r),
		Env:         parseKV(r, "env"),
		Credentials: parseKV(r, "cred"),
		Files:       parseFiles(r),
		Steps:       parseSteps(r),
		Access: flow.Access{
			Tools:       parseList(r, "tool"),
			Meta:        parseMeta(r),
			Skills:      parseList(r, "skill"),
			Credentials: parseList(r, "access_cred"),
			Flows:       r.Form["access_flow"],
		},
	}
}

func specToEditData(spec flow.Spec) flowEditData {
	argNames := make([]string, 0, len(spec.Args))
	for n := range spec.Args {
		argNames = append(argNames, n)
	}
	sort.Strings(argNames)
	args := make([]argRow, 0, len(argNames))
	for _, n := range argNames {
		a := spec.Args[n]
		args = append(args, argRow{Name: n, Default: a.Default, Description: a.Description, Required: a.Required})
	}

	files := make([]fileRow, 0, len(spec.Files))
	for _, f := range spec.Files {
		files = append(files, fileRow{Path: f.Path, Mode: f.Mode, Text: f.Text})
	}

	steps := make([]stepRow, 0, len(spec.Steps))
	for _, st := range spec.Steps {
		row := stepRow{
			Name:            st.Name,
			AllowedTools:    strings.Join(st.AllowedTools, " "),
			DisallowedTools: strings.Join(st.DisallowedTools, " "),
			Effort:          st.Effort,
			Model:           st.Model,
			PermissionMode:  st.PermissionMode,
			ContinueOnError: st.ContinueOn,
		}
		if st.MaxTurns != 0 {
			row.MaxTurns = strconv.Itoa(st.MaxTurns)
		}
		if st.Claude != "" {
			row.Type, row.Body = "claude", st.Claude
		} else {
			row.Type, row.Body = "run", st.Run
		}
		steps = append(steps, row)
	}

	cpus := ""
	if spec.Resources.CPUs != 0 {
		cpus = strconv.Itoa(spec.Resources.CPUs)
	}

	metaSet := toSet(spec.Access.Meta)
	return flowEditData{
		Name:        spec.Name,
		Description: spec.Description,
		Repo:        spec.Repo,
		Branch:      spec.Branch,
		Kranqfile:   spec.Kranqfile,
		Memory:      spec.Resources.Memory,
		CPUs:        cpus,
		Disk:        spec.Resources.Disk,

		Args:        withSpares(args, blankRows),
		Env:         withSpares(mapToRows(spec.Env), blankRows),
		Credentials: withSpares(mapToRows(spec.Credentials), blankRows),
		Files:       withSpares(files, blankRows),
		Steps:       withSpares(steps, blankSteps),

		AccessTools:          withSpares(append([]string{}, spec.Access.Tools...), blankRows),
		AccessMetaAsk:        metaSet["ask"],
		AccessMetaCronCreate: metaSet["cron.create"],
		AccessSkills:         withSpares(append([]string{}, spec.Access.Skills...), blankRows),
		AccessCredentials:    withSpares(append([]string{}, spec.Access.Credentials...), blankRows),
		AccessFlows:          toSet(spec.Access.Flows),
	}
}
