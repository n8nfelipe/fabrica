package workflow

import (
	"errors"
	"fmt"
	"strings"

	"fabrica/backend/internal/auth"
)

type State string
type Action string

const (
	StateOpen                State = "Aberta"
	StateReady               State = "Pronta"
	StateSpecifying          State = "Especificando"
	StateWaitingSpecApproval State = "Aguardando aprovação das especificações"
	StateSpecChanges         State = "Ajustes nas especificações"
	StateInDevelopment       State = "Em desenvolvimento"
	StateWaitingQA           State = "Aguardando QA"
	StateInQA                State = "Em QA"
	StateCorrectionNeeded    State = "Correção necessária"
	StateWaitingPR           State = "Aguardando PR"
	StatePROpen              State = "PR aberto"
	StateReadyForMerge       State = "Pronta para merge"
	StateBlocked             State = "Bloqueada"
	StateComplete            State = "Concluída"
	StatusNotStarted               = "not_started"
	StatusPending                  = "pending"
	StatusApproved                 = "approved"
	StatusChangesRequested         = "changes_requested"
	StatusSpecFailed               = "failed"
	StatusRunning                  = "running"
	StatusSucceeded                = "succeeded"
	StatusFailed                   = "failed"
	StatusRejected                 = "rejected"
	StatusNotCreated               = "not_created"
	StatusOpen                     = "open"
	StatusMerged                   = "merged"
	StatusUnknown                  = "unknown"
	StatusPassed                   = "passed"
	StatusChecksFailed             = "failed"
)

const (
	ActionMarkReady            Action = "mark_ready"
	ActionStartSpecification   Action = "start_specification"
	ActionSpecificationReady   Action = "specification_generated"
	ActionSpecificationFailed  Action = "specification_failed"
	ActionApproveSpecification Action = "approve_specification"
	ActionReturnSpecification  Action = "return_specification"
	ActionStartImplementation  Action = "implementation_started"
	ActionImplementationDone   Action = "implementation_succeeded"
	ActionImplementationError  Action = "implementation_failed"
	ActionStartQA              Action = "qa_started"
	ActionApproveQA            Action = "approve_qa"
	ActionRejectQA             Action = "reject_qa"
	ActionStartCorrection      Action = "start_correction"
	ActionCreatePR             Action = "pr_created"
	ActionChecksPending        Action = "checks_pending"
	ActionChecksFailed         Action = "checks_failed"
	ActionChecksPassed         Action = "checks_passed"
	ActionMerge                Action = "merge"
	ActionBlock                Action = "block"
	ActionUnblock              Action = "unblock"
)

var (
	ErrForbidden  = errors.New("role is not allowed to perform this action")
	ErrTransition = errors.New("action is not valid for the current task state")
)

type Task struct {
	State                State
	SpecificationStatus  string
	ImplementationStatus string
	QAStatus             string
	PRStatus             string
	ChecksStatus         string
	BlockedFromState     string
}

// Apply checks the actor and all phase gates before returning a new task state.
func Apply(task Task, action Action, role, comment, evidence string) (Task, error) {
	if !allowed(role, action) {
		return task, ErrForbidden
	}

	comment = strings.TrimSpace(comment)
	evidence = strings.TrimSpace(evidence)
	switch action {
	case ActionMarkReady:
		if task.State != StateOpen {
			return task, ErrTransition
		}
		task.State = StateReady
	case ActionStartSpecification:
		if task.State != StateReady && task.State != StateSpecChanges {
			return task, ErrTransition
		}
		task.State = StateSpecifying
		task.SpecificationStatus = StatusPending
	case ActionSpecificationReady:
		if task.State != StateSpecifying || evidence == "" {
			return task, ErrTransition
		}
		task.State = StateWaitingSpecApproval
		task.SpecificationStatus = StatusPending
	case ActionSpecificationFailed:
		if task.State != StateSpecifying || comment == "" {
			return task, ErrTransition
		}
		task.State = StateReady
		task.SpecificationStatus = StatusSpecFailed
	case ActionApproveSpecification:
		if task.State != StateWaitingSpecApproval || task.SpecificationStatus != StatusPending {
			return task, ErrTransition
		}
		task.State = StateInDevelopment
		task.SpecificationStatus = StatusApproved
		task.ImplementationStatus = StatusNotStarted
	case ActionReturnSpecification:
		if task.State != StateWaitingSpecApproval || task.SpecificationStatus != StatusPending || comment == "" {
			return task, ErrTransition
		}
		task.State = StateSpecChanges
		task.SpecificationStatus = StatusChangesRequested
	case ActionStartImplementation:
		if task.State != StateInDevelopment || task.SpecificationStatus != StatusApproved || (task.ImplementationStatus != StatusNotStarted && task.ImplementationStatus != StatusFailed) {
			return task, ErrTransition
		}
		task.ImplementationStatus = StatusRunning
	case ActionImplementationDone:
		if task.State != StateInDevelopment || task.ImplementationStatus != StatusRunning || evidence == "" {
			return task, ErrTransition
		}
		task.State = StateWaitingQA
		task.ImplementationStatus = StatusSucceeded
		task.QAStatus = StatusPending
	case ActionImplementationError:
		if task.State != StateInDevelopment || task.ImplementationStatus != StatusRunning || comment == "" {
			return task, ErrTransition
		}
		task.ImplementationStatus = StatusFailed
	case ActionStartQA:
		if task.State != StateWaitingQA || task.ImplementationStatus != StatusSucceeded || task.QAStatus != StatusPending {
			return task, ErrTransition
		}
		task.State = StateInQA
	case ActionApproveQA:
		if task.State != StateInQA || task.ImplementationStatus != StatusSucceeded || task.QAStatus != StatusPending || comment == "" || evidence == "" {
			return task, ErrTransition
		}
		task.State = StateWaitingPR
		task.QAStatus = StatusApproved
	case ActionRejectQA:
		if task.State != StateInQA || task.QAStatus != StatusPending || comment == "" || evidence == "" {
			return task, ErrTransition
		}
		task.State = StateCorrectionNeeded
		task.QAStatus = StatusRejected
	case ActionStartCorrection:
		if task.State != StateCorrectionNeeded || task.QAStatus != StatusRejected || task.SpecificationStatus != StatusApproved {
			return task, ErrTransition
		}
		task.State = StateInDevelopment
		task.ImplementationStatus = StatusRunning
		task.QAStatus = StatusNotStarted
	case ActionCreatePR:
		if task.State != StateWaitingPR || task.QAStatus != StatusApproved || evidence == "" {
			return task, ErrTransition
		}
		task.State = StatePROpen
		task.PRStatus = StatusOpen
	case ActionChecksPending:
		if (task.State != StatePROpen && task.State != StateReadyForMerge) || task.PRStatus != StatusOpen {
			return task, ErrTransition
		}
		task.State = StatePROpen
		task.ChecksStatus = StatusPending
	case ActionChecksFailed:
		if (task.State != StatePROpen && task.State != StateReadyForMerge) || task.PRStatus != StatusOpen {
			return task, ErrTransition
		}
		task.State = StatePROpen
		task.ChecksStatus = StatusChecksFailed
	case ActionChecksPassed:
		if (task.State != StatePROpen && task.State != StateReadyForMerge) || task.PRStatus != StatusOpen {
			return task, ErrTransition
		}
		task.State = StateReadyForMerge
		task.ChecksStatus = StatusPassed
	case ActionMerge:
		if task.State != StateReadyForMerge || task.PRStatus != StatusOpen || task.ChecksStatus != StatusPassed || task.QAStatus != StatusApproved {
			return task, ErrTransition
		}
		task.State = StateComplete
		task.PRStatus = StatusMerged
	case ActionBlock:
		if task.State == StateComplete || task.State == StateBlocked || comment == "" {
			return task, ErrTransition
		}
		task.BlockedFromState = string(task.State)
		task.State = StateBlocked
	case ActionUnblock:
		if task.State != StateBlocked || task.BlockedFromState == "" {
			return task, ErrTransition
		}
		task.State = State(task.BlockedFromState)
		task.BlockedFromState = ""
	default:
		return task, fmt.Errorf("unknown workflow action %q", action)
	}
	return task, nil
}

func allowed(role string, action Action) bool {
	if role == "system" {
		switch action {
		case ActionStartSpecification, ActionSpecificationReady, ActionSpecificationFailed, ActionCreatePR,
			ActionStartImplementation, ActionImplementationDone, ActionImplementationError,
			ActionChecksPending, ActionChecksFailed, ActionChecksPassed, ActionMerge:
			return true
		default:
			return false
		}
	}
	var policy auth.Action
	switch action {
	case ActionMarkReady, ActionStartSpecification:
		policy = auth.ActionOrganizeProjects
	case ActionSpecificationReady,
		ActionBlock, ActionUnblock:
		policy = auth.ActionManageUsers // Admin only; system uses its own branch above.
	case ActionApproveSpecification, ActionReturnSpecification:
		policy = auth.ActionApproveSpec
	case ActionStartImplementation, ActionImplementationDone, ActionImplementationError, ActionStartCorrection:
		policy = auth.ActionImplement
	case ActionStartQA, ActionApproveQA, ActionRejectQA:
		policy = auth.ActionPerformQA
	case ActionMerge:
		policy = auth.ActionMerge
	default:
		return false
	}
	return auth.Can(role, policy)
}
