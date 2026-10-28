package auth

import (
	"net/http/httptest"
	"testing"
)

func TestRequestIsLoopback(t *testing.T) {
	for _, remote := range []string{"127.0.0.1:1234", "[::1]:1234"} {
		r := httptest.NewRequest("POST", "/api/setup/admin", nil)
		r.RemoteAddr = remote
		if !requestIsLoopback(r) {
			t.Errorf("requestIsLoopback(%q) = false", remote)
		}
	}
	for _, remote := range []string{"192.0.2.1:1234", "invalid"} {
		r := httptest.NewRequest("POST", "/api/setup/admin", nil)
		r.RemoteAddr = remote
		if requestIsLoopback(r) {
			t.Errorf("requestIsLoopback(%q) = true", remote)
		}
	}
}

func TestValidateCredentials(t *testing.T) {
	if err := validateCredentials("first.admin", "long-enough-password"); err != nil {
		t.Fatalf("valid credentials rejected: %v", err)
	}
	for _, input := range []struct{ username, password string }{
		{"ab", "long-enough-password"},
		{"Bad Name", "long-enough-password"},
		{"valid.user", "short"},
	} {
		if err := validateCredentials(input.username, input.password); err == nil {
			t.Errorf("validateCredentials(%q, ...) accepted invalid input", input.username)
		}
	}
}

func TestRoleActionPolicy(t *testing.T) {
	cases := []struct {
		role   string
		action Action
		allow  bool
	}{
		{"Admin", ActionManageUsers, true},
		{"Admin", ActionMerge, true},
		{"PO", ActionApproveSpec, true},
		{"PO", ActionPerformQA, false},
		{"QA", ActionPerformQA, true},
		{"QA", ActionApproveSpec, false},
		{"Desenvolvedor", ActionImplement, true},
		{"Desenvolvedor", ActionMerge, true},
		{"Desenvolvedor", ActionManageUsers, false},
		{"Scrum Master", ActionOrganizeProjects, true},
		{"Scrum Master", ActionViewProgress, true},
		{"Scrum Master", ActionApproveSpec, false},
		{"Scrum Master", ActionPerformQA, false},
		{"Scrum Master", ActionMerge, false},
		{"unknown", ActionMerge, false},
	}
	for _, test := range cases {
		if got := Can(test.role, test.action); got != test.allow {
			t.Errorf("Can(%q, %q) = %t, want %t", test.role, test.action, got, test.allow)
		}
	}
}

func TestAvailableRoles(t *testing.T) {
	for _, role := range []string{"PO", "QA", "Admin", "Scrum Master", "Desenvolvedor"} {
		if !validRole(role) {
			t.Errorf("validRole(%q) = false", role)
		}
	}
	if validRole("Observer") {
		t.Fatal("validRole accepted an unsupported role")
	}
}
