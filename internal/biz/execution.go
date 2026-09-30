package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var (
	ErrInvalidAdmission  = errors.New("INVALID_ARGUMENT")
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
	if !validAdmissionID(a.TenantID) || !validAdmissionID(a.OperationID) || !validAdmissionID(a.ExecutionID) {
		return nil, nil, ErrInvalidAdmission
	}
	if a.Actor == "" || len(a.Actor) > 512 || !utf8.ValidString(a.Actor) || strings.TrimSpace(a.Actor) != a.Actor {
		return nil, nil, ErrInvalidAdmission
	}
	for _, character := range a.Actor {
		if unicode.IsControl(character) {
			return nil, nil, ErrInvalidAdmission
		}
	}
	acceptedAt := a.AcceptedAt.UTC()
	if acceptedAt.IsZero() || acceptedAt.Year() < 1 || acceptedAt.Year() > 9999 || acceptedAt.Nanosecond()%1000 != 0 || !acceptedAt.Before(a.Snapshot.DeadlineAt) {
		return nil, nil, ErrInvalidAdmission
	}
	intent, intentHash, err := cpup01.CanonicalIntent(a.Intent)
	if err != nil || intentHash != a.IntentHash {
		return nil, nil, ErrInvalidAdmission
	}
	snapshot, err := a.Snapshot.Canonical()
	if err != nil {
		return nil, nil, ErrInvalidAdmission
	}
	digest := sha256.Sum256(snapshot)
	if hex.EncodeToString(digest[:]) != a.SpecHash {
		return nil, nil, ErrInvalidAdmission
	}
	if a.Intent.Kind != a.Snapshot.Kind || !strings.EqualFold(a.Intent.PresetID, a.Snapshot.Release.PresetID) || !strings.EqualFold(a.Intent.DatasetVersionID, a.Snapshot.Input.InputVersionID) {
		return nil, nil, ErrInvalidAdmission
	}
	if a.Intent.ImageVersionID != nil && !strings.EqualFold(*a.Intent.ImageVersionID, a.Snapshot.Program.ImageVersionID) {
		return nil, nil, ErrInvalidAdmission
	}
	if a.Intent.GeneralParameters != nil {
		for _, parameter := range *a.Intent.GeneralParameters {
			matched := false
			for _, resolved := range a.Snapshot.Program.ResolvedParameters {
				if parameter.Name == resolved.Name && parameter.Type == resolved.Type {
					left, leftOK := new(big.Rat).SetString(parameter.Value)
					right, rightOK := new(big.Rat).SetString(resolved.Value)
					matched = leftOK && rightOK && left.Cmp(right) == 0
					break
				}
			}
			if !matched {
				return nil, nil, ErrInvalidAdmission
			}
		}
	}
	return intent, snapshot, nil
}

var admissionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validAdmissionID(value string) bool {
	return admissionIDPattern.MatchString(value) && value != "00000000-0000-0000-0000-000000000000"
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
