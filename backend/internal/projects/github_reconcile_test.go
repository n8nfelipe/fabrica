package projects

import (
	"testing"

	"fabrica/backend/internal/githubcli"
	"fabrica/backend/internal/workflow"
)

func TestCheckStatusRequiresEveryRequiredCheckToPass(t *testing.T) {
	cases := []struct {
		name   string
		checks []githubcli.Check
		want   string
	}{
		{name: "no required checks", want: workflow.StatusPending},
		{name: "pending check", checks: []githubcli.Check{{Name: "CI", State: "PENDING"}}, want: workflow.StatusPending},
		{name: "failed check dominates", checks: []githubcli.Check{{Name: "lint", State: "SUCCESS"}, {Name: "tests", State: "FAILURE"}}, want: workflow.StatusChecksFailed},
		{name: "all checks pass", checks: []githubcli.Check{{Name: "lint", State: "SUCCESS"}, {Name: "tests", State: "SUCCESS"}}, want: workflow.StatusPassed},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := checkStatus(test.checks); got != test.want {
				t.Fatalf("checkStatus()=%q want %q", got, test.want)
			}
		})
	}
}
