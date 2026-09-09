package cli

import (
	"fmt"

	"github.com/simon-em/kman/internal/catalog"
	"github.com/simon-em/kman/internal/exitcode"
	"github.com/simon-em/kman/internal/flow"
	"github.com/simon-em/kman/internal/skills"
)

func runValidate(env Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(env.Stderr, "usage: kman validate <flow.yaml>...")
		return exitcode.Usage
	}
	worst := exitcode.OK
	for _, path := range args {
		spec, code, err := loadFlow(path)
		if err != nil {
			fmt.Fprintf(env.Stderr, "%s: %v\n", path, err)
			worst = max(worst, code)
			continue
		}
		fmt.Fprintf(env.Stdout, "%s: ok (%d steps, %d args)\n", path, len(spec.Steps), len(spec.Args))
	}
	return worst
}

func runRender(env Env, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(env.Stderr, "usage: kman render <flow.yaml>")
		return exitcode.Usage
	}
	spec, code, err := loadFlow(args[0])
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: %v\n", args[0], err)
		return code
	}
	spec, err = skills.Compose(spec, catalog.Get)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: %v\n", args[0], err)
		return exitcode.InvalidSpec
	}
	out, err := flow.Render(spec)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: %v\n", args[0], err)
		return exitcode.InternalError
	}
	fmt.Fprint(env.Stdout, out)
	return exitcode.OK
}

func loadFlow(path string) (flow.Spec, int, error) {
	data, code, err := readFlowFile(path)
	if err != nil {
		return flow.Spec{}, code, err
	}
	spec, err := flow.Parse(data)
	if err != nil {
		return flow.Spec{}, exitcode.InvalidSpec, err
	}
	return spec, exitcode.OK, nil
}
