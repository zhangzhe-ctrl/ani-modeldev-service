package biz

import (
	"context"
	"errors"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var ErrNotImplemented = errors.New("NOT_IMPLEMENTED")

// Admission is the trusted, immutable command accepted by Governance. TenantID
// and Actor come from authenticated context, never from a user request body.
// Snapshot has no execution identity; this envelope supplies that binding.
type Admission struct {
	TenantID    string
	Actor       string
	OperationID string
	ExecutionID string
	Intent      cpup01.Intent
	IntentHash  string
	Snapshot    cpup01.Snapshot
	SpecHash    string
	AcceptedAt  time.Time
}

// Execution carries the persisted admission fact. Runtime resource bindings and
// observations are separate facts and must not rewrite this admission.
type Execution struct {
	Admission
}

// ExecutionRepository exposes only the persistence behaviors needed by the
// admission slice. Accept must commit the command before returning success.
type ExecutionRepository interface {
	Accept(context.Context, Admission) (Execution, error)
	Get(context.Context, string, string) (Execution, error)
}
