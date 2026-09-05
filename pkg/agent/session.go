package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/brandonapol/normal/pkg/config"
	"github.com/brandonapol/normal/pkg/engine"
)

type RejectionKind string

const (
	RejectPolicy          RejectionKind = "policy"
	RejectPatch           RejectionKind = "patch"
	RejectValidation      RejectionKind = "validation"
	RejectPlan            RejectionKind = "plan"
	RejectUnknownRevision RejectionKind = "unknown-revision"
	RejectQuotaExceeded   RejectionKind = "quota-exceeded"
)

type Rejection struct {
	Kind             RejectionKind      `json:"kind"`
	Message          string             `json:"message,omitempty"`
	PolicyIssues     []PolicyIssue      `json:"policyIssues,omitempty"`
	PatchIssues      []PatchIssue       `json:"patchIssues,omitempty"`
	ValidationIssues []config.Issue     `json:"validationIssues,omitempty"`
	PlanIssues       []engine.PlanIssue `json:"planIssues,omitempty"`
}

func (r *Rejection) Error() string {
	if r.Message != "" {
		return r.Message
	}
	messages := make([]string, 0)
	for _, issue := range r.PolicyIssues {
		messages = append(messages, issue.Message)
	}
	for _, issue := range r.PatchIssues {
		messages = append(messages, issue.Message)
	}
	for _, issue := range r.ValidationIssues {
		messages = append(messages, issue.Message)
	}
	for _, issue := range r.PlanIssues {
		messages = append(messages, issue.Message)
	}
	detail := joinMessages(messages)
	switch r.Kind {
	case RejectPolicy:
		return "the agent tool boundary refused this change: " + detail
	case RejectPatch:
		return "the patch could not be applied: " + detail
	case RejectValidation:
		return "the result would not be a valid config: " + detail
	case RejectPlan:
		return "the change cannot be planned: " + detail
	}
	return detail
}

type Evaluation struct {
	Desired          config.Config `json:"desired"`
	Diff             engine.Diff   `json:"diff"`
	Plan             engine.Plan   `json:"plan"`
	Review           []PolicyIssue `json:"review"`
	RequiresApproval bool          `json:"requiresApproval"`
}

func EvaluateOperations(current config.Config, operations []Operation, now time.Time) (Evaluation, *Rejection) {
	denied := make([]PolicyIssue, 0)
	for _, issue := range CheckOperations(operations) {
		if issue.Severity == SeverityDeny {
			denied = append(denied, issue)
		}
	}
	if len(denied) > 0 {
		return Evaluation{}, &Rejection{Kind: RejectPolicy, PolicyIssues: denied}
	}

	candidate, rejection := PatchConfig(current, operations, now)
	if rejection != nil {
		return Evaluation{}, rejection
	}
	return EvaluateConfig(current, candidate, operations)
}

func EvaluateConfig(current, candidate config.Config, operations []Operation) (Evaluation, *Rejection) {
	desired := engine.WithNextRevision(current, candidate)

	plan, err := engine.PlanApply(current, desired)
	if err != nil {
		var planErr *engine.PlanError
		if errors.As(err, &planErr) {
			return Evaluation{}, &Rejection{Kind: RejectPlan, PlanIssues: planErr.Issues}
		}
		return Evaluation{}, &Rejection{Kind: RejectPlan, Message: err.Error()}
	}

	verdict := CheckPolicy(operations, current, desired)
	if len(verdict.Denied) > 0 {
		return Evaluation{}, &Rejection{Kind: RejectPolicy, PolicyIssues: verdict.Denied}
	}

	return Evaluation{
		Desired:          desired,
		Diff:             plan.Diff,
		Plan:             plan,
		Review:           verdict.Review,
		RequiresApproval: verdict.RequiresApproval,
	}, nil
}

type ProposalStatus string

const (
	StatusPending   ProposalStatus = "pending"
	StatusApproved  ProposalStatus = "approved"
	StatusApplied   ProposalStatus = "applied"
	StatusDiscarded ProposalStatus = "discarded"
	StatusFailed    ProposalStatus = "failed"
)

type Proposal struct {
	ID         string         `json:"id"`
	Intent     string         `json:"intent"`
	CreatedAt  time.Time      `json:"createdAt"`
	Operations []Operation    `json:"operations"`
	Evaluation Evaluation     `json:"evaluation"`
	Status     ProposalStatus `json:"status"`
	ApprovedBy string         `json:"approvedBy,omitempty"`
}

type Revision struct {
	Revision      int           `json:"revision"`
	AppliedAt     time.Time     `json:"appliedAt"`
	TransactionID string        `json:"transactionId"`
	Intent        string        `json:"intent"`
	Config        config.Config `json:"config"`
}

type ApplyRejectionKind string

const (
	RejectUnknownProposal ApplyRejectionKind = "unknown-proposal"
	RejectNotApplicable   ApplyRejectionKind = "not-applicable"
	RejectApprovalNeeded  ApplyRejectionKind = "approval-required"
	RejectStale           ApplyRejectionKind = "stale"
	RejectApplyFailed     ApplyRejectionKind = "apply-failed"
	RejectApplyQuota      ApplyRejectionKind = "quota-exceeded"
)

type ApplyRejection struct {
	Kind      ApplyRejectionKind `json:"kind"`
	Message   string             `json:"message"`
	Rejection *Rejection         `json:"rejection,omitempty"`
	Failure   *engine.Failure    `json:"failure,omitempty"`
}

func (r *ApplyRejection) Error() string { return r.Message }

type Outcome struct {
	Proposal Proposal      `json:"proposal"`
	Report   engine.Report `json:"report"`
}

type SessionOptions struct {
	InitialConfig                 config.Config
	Ports                         engine.Ports
	ApprovalRequiredForEverything *bool
}

type Session struct {
	mu            sync.Mutex
	ports         engine.Ports
	current       config.Config
	counter       int
	applyAttempts int
	limits        config.Limits
	alwaysApprove bool
	proposals     map[string]Proposal
	order         []string
	history       []Revision
}

func NewSession(options SessionOptions) *Session {
	alwaysApprove := true
	if options.ApprovalRequiredForEverything != nil {
		alwaysApprove = *options.ApprovalRequiredForEverything
	}
	limits, err := config.SchemaLimits()
	if err != nil {
		limits = config.Limits{}
	}
	return &Session{
		ports:         options.Ports,
		current:       options.InitialConfig,
		limits:        limits,
		alwaysApprove: alwaysApprove,
		proposals:     make(map[string]Proposal),
		history: []Revision{{
			Revision:      options.InitialConfig.Metadata.Revision,
			AppliedAt:     options.Ports.Clock.Now(),
			TransactionID: "genesis",
			Intent:        "initial configuration",
			Config:        options.InitialConfig,
		}},
	}
}

func (s *Session) Current() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *Session) Revisions() []Revision {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Revision, len(s.history))
	copy(out, s.history)
	return out
}

func (s *Session) Proposals() []Proposal {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Proposal, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.proposals[id])
	}
	return out
}

func (s *Session) Proposal(id string) (Proposal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	proposal, ok := s.proposals[id]
	return proposal, ok
}

func (s *Session) record(intent string, operations []Operation, evaluation Evaluation) Proposal {
	s.counter++
	if s.alwaysApprove {
		evaluation.RequiresApproval = true
	}
	proposal := Proposal{
		ID:         fmt.Sprintf("proposal-%04d", s.counter),
		Intent:     intent,
		CreatedAt:  s.ports.Clock.Now(),
		Operations: operations,
		Evaluation: evaluation,
		Status:     StatusPending,
	}
	s.proposals[proposal.ID] = proposal
	s.order = append(s.order, proposal.ID)
	return proposal
}

func (s *Session) quotaExceeded() *Rejection {
	if s.limits.MaxProposalsPerSession > 0 && s.counter >= s.limits.MaxProposalsPerSession {
		return &Rejection{
			Kind: RejectQuotaExceeded,
			Message: fmt.Sprintf(
				"this session has already created %d proposals, which is its limit; start a new session",
				s.limits.MaxProposalsPerSession),
		}
	}
	return nil
}

func (s *Session) Propose(intent string, operations []Operation) (Proposal, *Rejection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if quota := s.quotaExceeded(); quota != nil {
		return Proposal{}, quota
	}
	evaluation, rejection := EvaluateOperations(s.current, operations, s.ports.Clock.Now())
	if rejection != nil {
		return Proposal{}, rejection
	}
	return s.record(intent, operations, evaluation), nil
}

func (s *Session) ProposeRollback(revision int) (Proposal, *Rejection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if quota := s.quotaExceeded(); quota != nil {
		return Proposal{}, quota
	}

	var target *Revision
	for i := range s.history {
		if s.history[i].Revision == revision {
			target = &s.history[i]
			break
		}
	}
	if target == nil {
		return Proposal{}, &Rejection{
			Kind:    RejectUnknownRevision,
			Message: fmt.Sprintf("no applied revision %d", revision),
		}
	}

	evaluation, rejection := EvaluateConfig(s.current, target.Config, nil)
	if rejection != nil {
		return Proposal{}, rejection
	}
	evaluation.RequiresApproval = true
	return s.record(fmt.Sprintf("roll back to revision %d", revision), nil, evaluation), nil
}

func (s *Session) Approve(id, actor string) (Proposal, *ApplyRejection) {
	s.mu.Lock()
	defer s.mu.Unlock()

	proposal, ok := s.proposals[id]
	if !ok {
		return Proposal{}, &ApplyRejection{
			Kind: RejectUnknownProposal, Message: fmt.Sprintf("no proposal %q", id),
		}
	}
	if proposal.Status != StatusPending {
		return Proposal{}, &ApplyRejection{
			Kind: RejectNotApplicable, Message: fmt.Sprintf("proposal %q is %s", id, proposal.Status),
		}
	}
	proposal.Status = StatusApproved
	proposal.ApprovedBy = actor
	s.proposals[id] = proposal
	return proposal, nil
}

func (s *Session) Discard(id string) (Proposal, *ApplyRejection) {
	s.mu.Lock()
	defer s.mu.Unlock()

	proposal, ok := s.proposals[id]
	if !ok {
		return Proposal{}, &ApplyRejection{
			Kind: RejectUnknownProposal, Message: fmt.Sprintf("no proposal %q", id),
		}
	}
	if proposal.Status == StatusApplied {
		return Proposal{}, &ApplyRejection{
			Kind: RejectNotApplicable, Message: "an applied proposal cannot be discarded",
		}
	}
	proposal.Status = StatusDiscarded
	s.proposals[id] = proposal
	return proposal, nil
}

func (s *Session) Apply(ctx context.Context, id string) (Outcome, *ApplyRejection) {
	s.mu.Lock()
	defer s.mu.Unlock()

	proposal, ok := s.proposals[id]
	if !ok {
		return Outcome{}, &ApplyRejection{
			Kind: RejectUnknownProposal, Message: fmt.Sprintf("no proposal %q", id),
		}
	}
	if proposal.Status == StatusDiscarded || proposal.Status == StatusApplied {
		return Outcome{}, &ApplyRejection{
			Kind: RejectNotApplicable, Message: fmt.Sprintf("proposal %q is %s", id, proposal.Status),
		}
	}
	if proposal.Evaluation.RequiresApproval && proposal.Status != StatusApproved {
		return Outcome{}, &ApplyRejection{
			Kind:    RejectApprovalNeeded,
			Message: fmt.Sprintf("proposal %q changes policy the user must confirm before it is applied", id),
		}
	}
	if s.limits.MaxAppliesPerSession > 0 && s.applyAttempts >= s.limits.MaxAppliesPerSession {
		return Outcome{}, &ApplyRejection{
			Kind: RejectApplyQuota,
			Message: fmt.Sprintf(
				"this session has already attempted %d applies, which is its limit; start a new session",
				s.limits.MaxAppliesPerSession),
		}
	}
	s.applyAttempts++

	var (
		fresh     Evaluation
		rejection *Rejection
	)
	if len(proposal.Operations) > 0 {
		fresh, rejection = EvaluateOperations(s.current, proposal.Operations, s.ports.Clock.Now())
	} else {
		fresh, rejection = EvaluateConfig(s.current, proposal.Evaluation.Desired, nil)
	}
	if rejection != nil {
		return Outcome{}, &ApplyRejection{
			Kind:      RejectStale,
			Message:   "the config moved since this proposal was written: " + rejection.Error(),
			Rejection: rejection,
		}
	}

	plan := fresh.Plan
	plan.Intent = proposal.Intent
	plan.ApprovedBy = proposal.ApprovedBy

	report, err := engine.ApplyPlan(ctx, plan, s.ports)
	if err != nil {
		proposal.Status = StatusFailed
		s.proposals[id] = proposal
		var failure *engine.Failure
		if !errors.As(err, &failure) {
			return Outcome{}, &ApplyRejection{Kind: RejectApplyFailed, Message: err.Error()}
		}
		return Outcome{}, &ApplyRejection{
			Kind: RejectApplyFailed, Message: failure.Error(), Failure: failure,
		}
	}

	s.current = fresh.Desired
	s.history = append(s.history, Revision{
		Revision:      s.current.Metadata.Revision,
		AppliedAt:     report.FinishedAt,
		TransactionID: report.TransactionID,
		Intent:        proposal.Intent,
		Config:        s.current,
	})

	proposal.Status = StatusApplied
	proposal.Evaluation = fresh
	s.proposals[id] = proposal
	return Outcome{Proposal: proposal, Report: report}, nil
}

func ApprovalNotRequiredForEverything() *bool {
	value := false
	return &value
}
