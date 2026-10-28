package workflow

import (
	"errors"
	"testing"
)

func TestTaskWorkflowRequiresEachGate(t *testing.T) {
	task := Task{
		State: StateOpen, SpecificationStatus: StatusNotStarted,
		ImplementationStatus: StatusNotStarted, QAStatus: StatusNotStarted,
		PRStatus: StatusNotCreated, ChecksStatus: StatusUnknown,
	}
	apply := func(action Action, role, comment, evidence string) {
		t.Helper()
		updated, err := Apply(task, action, role, comment, evidence)
		if err != nil {
			t.Fatalf("Apply(%q, %q) failed: %v", action, role, err)
		}
		task = updated
	}
	denied := func(action Action, role string, want error) {
		t.Helper()
		if _, err := Apply(task, action, role, "", ""); !errors.Is(err, want) {
			t.Fatalf("Apply(%q, %q) error = %v, want %v", action, role, err, want)
		}
	}

	denied(ActionMerge, "Admin", ErrTransition)
	denied(ActionMarkReady, "QA", ErrForbidden)
	apply(ActionMarkReady, "Scrum Master", "", "")
	denied(ActionApproveSpecification, "Scrum Master", ErrForbidden)
	apply(ActionStartSpecification, "system", "", "")
	denied(ActionApproveSpecification, "PO", ErrTransition)
	apply(ActionSpecificationReady, "system", "", "openspec/tasks.md")
	apply(ActionApproveSpecification, "PO", "", "")
	apply(ActionStartImplementation, "Desenvolvedor", "", "")
	apply(ActionImplementationDone, "system", "", "run-42")
	denied(ActionApproveQA, "QA", ErrTransition)
	apply(ActionStartQA, "QA", "", "")
	apply(ActionApproveQA, "QA", "Fluxo principal revisado", "http://127.0.0.1:41000")
	denied(ActionMerge, "Desenvolvedor", ErrTransition)
	apply(ActionCreatePR, "system", "", "https://github.com/org/repo/pull/7")
	apply(ActionChecksPending, "system", "", "")
	denied(ActionMerge, "PO", ErrTransition)
	apply(ActionChecksFailed, "system", "", "")
	denied(ActionMerge, "QA", ErrTransition)
	apply(ActionChecksPassed, "system", "", "")
	apply(ActionMerge, "Desenvolvedor", "", "")
	if task.State != StateComplete || task.PRStatus != StatusMerged {
		t.Fatalf("final task = %#v, want complete and merged", task)
	}
	denied(ActionBlock, "Admin", ErrTransition)
}

func TestQADecisionRequiresCommentAndEvidence(t *testing.T) {
	task := Task{State: StateInQA, ImplementationStatus: StatusSucceeded, QAStatus: StatusPending}
	for _, test := range []struct{ comment, evidence string }{{"", "url"}, {"ok", ""}} {
		if _, err := Apply(task, ActionApproveQA, "QA", test.comment, test.evidence); !errors.Is(err, ErrTransition) {
			t.Errorf("approval with comment=%q evidence=%q returned %v", test.comment, test.evidence, err)
		}
	}
	if _, err := Apply(task, ActionRejectQA, "QA", "Needs changes", "screenshot unavailable"); err != nil {
		t.Fatalf("rejection with recorded evidence should succeed: %v", err)
	}
}

func TestStartCorrectionStartsImplementationWithDeveloperPermission(t *testing.T) {
	task := Task{State: StateCorrectionNeeded, SpecificationStatus: StatusApproved, QAStatus: StatusRejected, ImplementationStatus: StatusSucceeded}
	updated, err := Apply(task, ActionStartCorrection, "Desenvolvedor", "", "")
	if err != nil {
		t.Fatalf("start correction failed: %v", err)
	}
	if updated.State != StateInDevelopment || updated.ImplementationStatus != StatusRunning || updated.QAStatus != StatusNotStarted {
		t.Fatalf("correction state = %#v, want development with running implementation and pending QA", updated)
	}
	if _, err := Apply(task, ActionStartCorrection, "QA", "", ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("QA starting a correction error = %v, want ErrForbidden", err)
	}
	if _, err := Apply(Task{State: StateInQA, SpecificationStatus: StatusApproved, QAStatus: StatusPending}, ActionStartCorrection, "Desenvolvedor", "", ""); !errors.Is(err, ErrTransition) {
		t.Fatalf("starting correction outside correction state error = %v, want ErrTransition", err)
	}
}

func TestQACorrectionMustReturnForNewApprovalBeforePR(t *testing.T) {
	task := Task{State: StateInQA, SpecificationStatus: StatusApproved, ImplementationStatus: StatusSucceeded, QAStatus: StatusPending}
	var err error
	task, err = Apply(task, ActionRejectQA, "QA", "Corrigir o fluxo de login", "Falha reproduzida no passo 2")
	if err != nil {
		t.Fatalf("reject QA: %v", err)
	}
	task, err = Apply(task, ActionStartCorrection, "Desenvolvedor", "", "")
	if err != nil {
		t.Fatalf("start correction: %v", err)
	}
	task, err = Apply(task, ActionImplementationDone, "system", "", "login.go")
	if err != nil || task.State != StateWaitingQA || task.QAStatus != StatusPending {
		t.Fatalf("successful correction = %#v, %v; want QA pending", task, err)
	}
	if _, err := Apply(task, ActionCreatePR, "system", "", "https://github.com/acme/app/pull/1"); !errors.Is(err, ErrTransition) {
		t.Fatalf("PR before new QA approval error = %v, want ErrTransition", err)
	}
	task, err = Apply(task, ActionStartQA, "QA", "", "")
	if err != nil {
		t.Fatalf("restart QA: %v", err)
	}
	task, err = Apply(task, ActionApproveQA, "QA", "Correção validada", "http://127.0.0.1:41000")
	if err != nil || task.State != StateWaitingPR {
		t.Fatalf("approve corrected work = %#v, %v; want PR waiting", task, err)
	}
}

func TestFailedQACorrectionCanBeRetried(t *testing.T) {
	task := Task{State: StateInQA, SpecificationStatus: StatusApproved, ImplementationStatus: StatusSucceeded, QAStatus: StatusPending}
	var err error
	task, err = Apply(task, ActionRejectQA, "QA", "Corrigir o fluxo de login", "Falha reproduzida no passo 2")
	if err != nil {
		t.Fatalf("reject QA: %v", err)
	}
	task, err = Apply(task, ActionStartCorrection, "Desenvolvedor", "", "")
	if err != nil {
		t.Fatalf("start correction: %v", err)
	}
	task, err = Apply(task, ActionImplementationError, "system", "agent failed", "")
	if err != nil || task.State != StateInDevelopment || task.ImplementationStatus != StatusFailed {
		t.Fatalf("failed correction = %#v, %v; want failed in development", task, err)
	}
	task, err = Apply(task, ActionStartImplementation, "Desenvolvedor", "", "")
	if err != nil || task.ImplementationStatus != StatusRunning {
		t.Fatalf("retry correction = %#v, %v; want running implementation", task, err)
	}
}

func TestSpecificationFailureCanRetryButCannotImplement(t *testing.T) {
	task, err := Apply(Task{State: StateOpen}, ActionMarkReady, "PO", "", "")
	if err != nil {
		t.Fatal(err)
	}
	task, err = Apply(task, ActionStartSpecification, "system", "", "")
	if err != nil {
		t.Fatal(err)
	}
	task, err = Apply(task, ActionSpecificationFailed, "system", "generation failed", "")
	if err != nil {
		t.Fatal(err)
	}
	if task.State != StateReady || task.SpecificationStatus != StatusSpecFailed {
		t.Fatalf("failed specification should be retryable from ready: %#v", task)
	}
	if _, err := Apply(task, ActionStartImplementation, "Desenvolvedor", "", ""); !errors.Is(err, ErrTransition) {
		t.Fatalf("implementation before PO approval should stay blocked, got %v", err)
	}
	task, err = Apply(task, ActionStartSpecification, "Scrum Master", "", "")
	if err != nil || task.State != StateSpecifying || task.SpecificationStatus != StatusPending {
		t.Fatalf("authorized retry should restart specification: %#v, err %v", task, err)
	}
}

func TestTaskWorkflowRequiresCommentsForReturnsAndBlocks(t *testing.T) {
	task := Task{State: StateWaitingSpecApproval, SpecificationStatus: StatusPending}
	if _, err := Apply(task, ActionReturnSpecification, "PO", "", ""); !errors.Is(err, ErrTransition) {
		t.Fatalf("return without comment error = %v, want ErrTransition", err)
	}
	returned, err := Apply(task, ActionReturnSpecification, "PO", "Clarify acceptance criteria", "")
	if err != nil || returned.State != StateSpecChanges {
		t.Fatalf("return with comment = %#v, %v", returned, err)
	}
	blocked, err := Apply(returned, ActionBlock, "Admin", "Waiting for a decision", "")
	if err != nil || blocked.State != StateBlocked || blocked.BlockedFromState != string(StateSpecChanges) {
		t.Fatalf("block = %#v, %v", blocked, err)
	}
	unblocked, err := Apply(blocked, ActionUnblock, "Admin", "", "")
	if err != nil || unblocked.State != StateSpecChanges || unblocked.BlockedFromState != "" {
		t.Fatalf("unblock = %#v, %v", unblocked, err)
	}
}

func TestScrumMasterHasOnlyOrganizationAndProgressPermissions(t *testing.T) {
	for _, action := range []Action{
		ActionApproveSpecification, ActionReturnSpecification,
		ActionStartImplementation, ActionImplementationDone, ActionImplementationError,
		ActionStartQA, ActionApproveQA, ActionRejectQA, ActionMerge,
	} {
		if _, err := Apply(Task{State: StateOpen}, action, "Scrum Master", "reason", "evidence"); !errors.Is(err, ErrForbidden) {
			t.Errorf("Scrum Master action %q error = %v, want ErrForbidden", action, err)
		}
	}
}
