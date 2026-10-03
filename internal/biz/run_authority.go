package biz

import (
	"errors"
	"time"
)

var ErrRunAuthorityConflict = errors.New("RUN_AUTHORITY_CONFLICT")

// RunAuthorityCandidate is an internal association already checked by the
// upstream workload identity adapter. It is not a public request, and merely
// constructing it proves neither that identity check nor training permission.
// The repository must still match it to the committed admission and dispatch.
type RunAuthorityCandidate struct {
	TenantID      string
	ExecutionID   string
	OperationID   string
	SpecHash      string
	AttemptID     string
	PlanHash      string
	RunID         string
	NamespaceName string
	NamespaceUID  string
	WorkflowName  string
	WorkflowUID   string
}

// RunAuthority retains one immutable Run/Workflow association for an execution.
// BoundAt is its first database binding time. OwnerRevision describes the current
// committed execution aggregate; neither field grants permission to create work.
type RunAuthority struct {
	RunAuthorityCandidate
	BoundAt       time.Time
	OwnerRevision uint64
}

// RunAuthorityReceipt is returned only after the binding transaction commits.
// Replayed identifies an exact association replay, never a new authority grant.
type RunAuthorityReceipt struct {
	RunAuthority
	Replayed bool
}
