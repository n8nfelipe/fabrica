package securitygateway

import (
	"fmt"
	"strings"
	"testing"
)

func TestNewPolicyRejectsInvalidRootsPhasesAndScopes(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		root, phase string
		taskID      int64
		paths       []string
	}{
		{"", PhaseSpecification, 1, nil},
		{root, "unknown", 1, nil},
		{root, PhaseSpecification, 0, nil},
		{root, PhaseImplementation, 1, nil},
		{root, PhaseImplementation, 1, []string{"src/", "src/"}},
		{root, PhaseImplementation, 1, []string{"src/*"}},
		{root, PhaseImplementation, 1, []string{"../outside"}},
		{root, PhaseImplementation, 1, []string{".env"}},
		{root, PhaseImplementation, 1, []string{"src/" + strings.Repeat("x", 1025)}},
	} {
		if _, err := NewPolicy(tc.root, tc.phase, tc.taskID, tc.paths, ""); err == nil {
			t.Errorf("expected invalid policy to fail: %#v", tc)
		}
	}
	if _, err := NewPolicy(root, PhaseImplementation, 1, []string{"src/"}, ""); err != nil {
		t.Fatalf("valid implementation policy failed: %v", err)
	}
	tooMany := make([]string, MaxScopePaths+1)
	for i := range tooMany {
		tooMany[i] = "src/path" + strings.Repeat("a", i) + ".go"
	}
	if _, err := NewPolicy(root, PhaseImplementation, 1, tooMany, ""); err == nil {
		t.Fatal("expected excessive number of scope paths to fail")
	}
	longScope := make([]string, 40)
	for i := range longScope {
		longScope[i] = fmt.Sprintf("%02d", i) + strings.Repeat(string(rune('a'+i%26)), 900)
	}
	if _, err := NewPolicy(root, PhaseImplementation, 1, longScope, ""); err == nil {
		t.Fatal("expected excessive aggregate scope length to fail")
	}
}

func TestStringListFlagValue(t *testing.T) {
	var values stringList
	if err := values.Set("src/"); err != nil {
		t.Fatal(err)
	}
	if err := values.Set("README.md"); err != nil {
		t.Fatal(err)
	}
	if values.String() != "[src/ README.md]" {
		t.Fatalf("unexpected flag string value %q", values.String())
	}
}
