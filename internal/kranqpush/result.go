package kranqpush

import (
	"strconv"
	"strings"
)

const ResultMarker = "KRANQ-RESULT"

const StatusRefused = "refused"

type Result struct {
	Found     bool
	ID        string
	Status    string
	ExitCode  int
	ResultRef string
}

func ParseResult(output string) Result {
	var res Result
	for _, line := range strings.Split(output, "\n") {
		idx := strings.Index(line, ResultMarker+" ")
		if idx < 0 {
			continue
		}
		res = Result{Found: true}
		for _, field := range strings.Fields(line[idx+len(ResultMarker):]) {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			switch key {
			case "id":
				res.ID = value
			case "status":
				res.Status = value
			case "result":
				res.ResultRef = value
			case "exit":
				if n, err := strconv.Atoi(value); err == nil {
					res.ExitCode = n
				}
			}
		}
	}
	return res
}
