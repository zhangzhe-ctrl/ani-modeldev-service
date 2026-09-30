package biz

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var (
	ErrInvalidAdmission  = errors.New("INVALID_ARGUMENT")
	ErrAdmissionConflict = errors.New("ADMISSION_CONFLICT")
	ErrExecutionNotFound = errors.New("NOT_FOUND")
	ErrPersistence       = errors.New("PERSISTENCE_UNAVAILABLE")
)

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

// CanonicalPayloads validates the immutable envelope and returns the exact
// bytes to persist. It never resolves a current Release or compares with the
// wall clock: a delayed delivery retains its original accepted_at/deadline.
func (a Admission) CanonicalPayloads() ([]byte, []byte, error) {
	intent, snapshot, err := cpup01.AdmissionEnvelope(a).CanonicalPayloads()
	if err != nil {
		return nil, nil, ErrInvalidAdmission
	}
	return intent, snapshot, nil
}

var admissionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validAdmissionID(value string) bool {
	return admissionIDPattern.MatchString(value) && value != "00000000-0000-0000-0000-000000000000"
}

func validAuditActor(value string) bool {
	return cpup01.ValidAuditActor(value)
}

// Execution carries the persisted admission fact. Runtime resource bindings and
// observations are separate facts and must not rewrite this admission.
type Execution struct {
	Admission
	// Close is an observed durable fact, never a creation permit. A nil value
	// does not authorize work; a creator must check the shared identity lock.
	Close *CloseRecord
}

type CloseReason string

const CloseReasonUserStop CloseReason = "USER_STOP"

type CloseState string

const CloseStateClosing CloseState = "CLOSING"

// CloseIntent is a trusted Governance USER_STOP delivery. SourceGeneration is
// the sender's deduplication sequence, never the ModelDev creation fence.
// Identity and SpecHash are known even if Admission delivery has not arrived.
type CloseIntent struct {
	TenantID         string
	OperationID      string
	ExecutionID      string
	SpecHash         string
	SourceGeneration uint64
	Reason           CloseReason
	RequestedAt      time.Time
	RequestedActor   string
}

var closeSpecHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (intent CloseIntent) Validate() error {
	if !validAdmissionID(intent.TenantID) || !validAdmissionID(intent.OperationID) || !validAdmissionID(intent.ExecutionID) || !closeSpecHashPattern.MatchString(intent.SpecHash) {
		return ErrInvalidAdmission
	}
	if intent.SourceGeneration == 0 || intent.Reason != CloseReasonUserStop || !validAuditActor(intent.RequestedActor) {
		return ErrInvalidAdmission
	}
	requestedAt := intent.RequestedAt.UTC()
	if requestedAt.IsZero() || requestedAt.Year() < 1 || requestedAt.Year() > 9999 || requestedAt.Nanosecond()%1000 != 0 {
		return ErrInvalidAdmission
	}
	return nil
}

// CloseRecord carries both the original source sequence and the owner fence.
// ModelDev alone allocates Generation for this execution across close sources;
// a replay keeps the original Generation. CLOSING is not proof of no writers.
type CloseRecord struct {
	CloseIntent
	Generation uint64
	State      CloseState
}

// ExecutionRepository exposes only the persistence behaviors needed by the
// admission and first close-intent slices. Successful command receipts require
// a durable commit. No runtime resource operation is implied by this port.
type ExecutionRepository interface {
	Accept(context.Context, Admission) (Execution, error)
	Get(context.Context, string, string) (Execution, error)
	ApplyCloseIntent(context.Context, CloseIntent) (CloseRecord, error)
	GetCloseIntent(context.Context, string, string) (CloseRecord, error)
}
