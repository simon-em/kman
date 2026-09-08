package kranqpush

import "testing"

func TestParseResultFound(t *testing.T) {
	output := "some git noise\nKRANQ-RESULT id=abc123 status=ok exit=0 result=refs/heads/ok/abc123\nmore noise\n"
	res := ParseResult(output)
	if !res.Found {
		t.Fatal("expected Found = true")
	}
	if res.ID != "abc123" || res.Status != "ok" || res.ExitCode != 0 || res.ResultRef != "refs/heads/ok/abc123" {
		t.Errorf("res = %+v", res)
	}
}

func TestParseResultNotFound(t *testing.T) {
	res := ParseResult("just some ordinary git push output\n")
	if res.Found {
		t.Errorf("res.Found = true, want false: %+v", res)
	}
}

func TestParseResultDoesNotMatchLongerWord(t *testing.T) {
	res := ParseResult("KRANQ-RESULTS id=abc status=ok exit=0\n")
	if res.Found {
		t.Errorf("res.Found = true, want false: a longer word must not match")
	}
}

func TestParseResultTakesTheLastLine(t *testing.T) {
	output := "KRANQ-RESULT id=first status=ok exit=0\nKRANQ-RESULT id=second status=failed exit=1\n"
	res := ParseResult(output)
	if res.ID != "second" || res.Status != "failed" || res.ExitCode != 1 {
		t.Errorf("res = %+v, want the last line to win", res)
	}
}

func TestParseResultNonNumericExitIsIgnored(t *testing.T) {
	res := ParseResult("KRANQ-RESULT id=abc status=ok exit=not-a-number\n")
	if !res.Found || res.ExitCode != 0 {
		t.Errorf("res = %+v", res)
	}
}
