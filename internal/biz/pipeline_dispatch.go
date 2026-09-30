package biz

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

// PipelineOwnerConfiguration is an explicitly supplied owner configuration
// revision. Its digest is a reference, not proof that the configuration exists
// or that the control workload may use its storage. It contains no credentials.
type PipelineOwnerConfiguration struct {
	Reference      string `json:"reference"`
	RevisionSHA256 string `json:"revision_sha256"`
	PipelineRoot   string `json:"pipeline_root"`
}

type PipelineDispatchRequest struct {
	Admission Admission
	Owner     PipelineOwnerConfiguration
}

// PipelineDispatchPlan is internal owner state, separate from the shared
// execution snapshot. Admission.SpecHash does not cover Owner.PipelineRoot.
// Repositories accept a request and derive this plan from its validated
// admission; a caller-supplied plan is never an admission replacement.
type PipelineDispatchPlan struct {
	SchemaVersion     string                            `json:"schema_version"`
	TenantID          string                            `json:"tenant_id"`
	ExecutionID       string                            `json:"execution_id"`
	OperationID       string                            `json:"operation_id"`
	SpecHash          string                            `json:"spec_hash"`
	Owner             PipelineOwnerConfiguration        `json:"owner"`
	Environment       cpup01.EnvironmentBindingSnapshot `json:"environment"`
	PipelineID        string                            `json:"pipeline_id"`
	PipelineVersionID string                            `json:"pipeline_version_id"`
	DisplayName       string                            `json:"display_name"`
	DeadlineAt        time.Time                         `json:"deadline_at"`
}

var pipelineOwnerReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,511}$`)

// Freeze derives service account, experiment, pipeline identities and the
// execution/spec parameters exclusively from the existing admission contract.
// The repository must additionally compare that entire admission with its
// committed original under the shared execution identity lock.
func (request PipelineDispatchRequest) Freeze() (PipelineDispatchPlan, error) {
	_, snapshotBytes, err := request.Admission.CanonicalPayloads()
	if err != nil {
		return PipelineDispatchPlan{}, err
	}
	owner := request.Owner
	if !pipelineOwnerReferencePattern.MatchString(owner.Reference) || strings.Contains(owner.Reference, "://") || !closeSpecHashPattern.MatchString(owner.RevisionSHA256) {
		return PipelineDispatchPlan{}, ErrInvalidAdmission
	}
	root, err := url.Parse(owner.PipelineRoot)
	if err != nil || root.Scheme != "s3" || root.Host == "" || root.User != nil || root.Port() != "" || root.RawQuery != "" || root.ForceQuery || root.Fragment != "" || root.RawPath != "" || root.Path == "" || path.Clean(root.Path) != strings.TrimSuffix(root.Path, "/") {
		return PipelineDispatchPlan{}, ErrInvalidAdmission
	}
	var snapshot cpup01.Snapshot
	if json.Unmarshal(snapshotBytes, &snapshot) != nil {
		return PipelineDispatchPlan{}, ErrInvalidAdmission
	}
	return PipelineDispatchPlan{
		SchemaVersion: "ani.modeldev.pipeline-dispatch-plan.v1",
		TenantID:      strings.ToLower(request.Admission.TenantID),
		ExecutionID:   strings.ToLower(request.Admission.ExecutionID),
		OperationID:   strings.ToLower(request.Admission.OperationID),
		SpecHash:      request.Admission.SpecHash,
		Owner:         owner, Environment: snapshot.Environment,
		PipelineID: snapshot.Release.PipelineID, PipelineVersionID: snapshot.Release.PipelineVersionID,
		DisplayName: "md-" + strings.ToLower(request.Admission.ExecutionID),
		DeadlineAt:  snapshot.DeadlineAt,
	}, nil
}

// Canonical encodes the internal plan in field order with no HTML escapes or
// trailing newline. Only Freeze establishes its relationship to an admission.
func (plan PipelineDispatchPlan) Canonical() ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(plan); err != nil {
		return nil, ErrInvalidAdmission
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func (plan PipelineDispatchPlan) Digest() (string, error) {
	canonical, err := plan.Canonical()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

type PipelineDispatchState string

const PipelineDispatchSubmitting PipelineDispatchState = "SUBMITTING"

type PipelineDispatch struct {
	AttemptID  string
	Plan       PipelineDispatchPlan
	PlanHash   string
	State      PipelineDispatchState
	ReservedAt time.Time
}

// PipelineSendPermit is returned only by the transaction which creates and
// successfully commits the first reservation. Reading a persisted dispatch or
// replaying Reserve must never reconstruct this permission.
type PipelineSendPermit struct {
	TenantID    string
	ExecutionID string
	AttemptID   string
	PlanHash    string
}

type PipelineDispatchReservation struct {
	Dispatch   PipelineDispatch
	SendPermit *PipelineSendPermit
}

type PipelineDispatchRepository interface {
	Reserve(context.Context, PipelineDispatchRequest) (PipelineDispatchReservation, error)
	Get(context.Context, string, string) (PipelineDispatch, error)
}
