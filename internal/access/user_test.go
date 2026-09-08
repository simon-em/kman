package access

import "testing"

func TestParseUserValid(t *testing.T) {
	u, err := ParseUser([]byte("id: simon\ndisplay_name: Simon\nslack_user_id: U123\nflows: [deploy-review]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "simon" || u.SlackUserID != "U123" || len(u.Flows) != 1 {
		t.Errorf("u = %+v", u)
	}
}

func TestParseUserRejectsMissingID(t *testing.T) {
	if _, err := ParseUser([]byte("display_name: Simon\n")); err == nil {
		t.Fatal("expected an error for a user with no id")
	}
}

func TestParseUserRejectsEmptyFlowEntry(t *testing.T) {
	if _, err := ParseUser([]byte("id: simon\nflows: [\"\"]\n")); err == nil {
		t.Fatal("expected an error for an empty flows entry")
	}
}
