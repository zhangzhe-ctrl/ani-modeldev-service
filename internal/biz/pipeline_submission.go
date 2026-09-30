package biz

import (
	"bytes"
	"context"
	"strings"
)

// PipelineSubmissionState describes an outbound call observation, not a
// persisted compute state, an authoritative Run, or a training creation permit.
type PipelineSubmissionState string

const (
	PipelineSubmissionNotSent   PipelineSubmissionState = "NOT_SENT"
	PipelineSubmissionUncertain PipelineSubmissionState = "UNCERTAIN"
	PipelineSubmissionConfirmed PipelineSubmissionState = "CONFIRMED"
)

type PipelineSubmissionObservation struct {
	State PipelineSubmissionState
	RunID string
}

// PipelineCreateRequest carries the original admission and the plan/permit
// returned by one committed reservation. These constructible values are not
// proof of persistence or current authorization; the submitter owns their origin.
type PipelineCreateRequest struct {
	Admission Admission
	Plan      PipelineDispatchPlan
	Permit    PipelineSendPermit
}

// Validate checks the complete canonical admission-to-plan association and the
// permit's internal identity. Only the repository can verify the stored original
// admission and issue the original attempt after its transaction commits.
func (request PipelineCreateRequest) Validate() error {
	plan, err := (PipelineDispatchRequest{Admission: request.Admission, Owner: request.Plan.Owner}).Freeze()
	if err != nil {
		return err
	}
	wanted, err := plan.Canonical()
	if err != nil {
		return err
	}
	actual, err := request.Plan.Canonical()
	if err != nil || !bytes.Equal(actual, wanted) {
		return ErrInvalidAdmission
	}
	hash, err := plan.Digest()
	permit := request.Permit
	if err != nil || hash != permit.PlanHash || !validAdmissionID(permit.TenantID) || !validAdmissionID(permit.ExecutionID) || !validAdmissionID(permit.AttemptID) ||
		!strings.EqualFold(permit.TenantID, plan.TenantID) || !strings.EqualFold(permit.ExecutionID, plan.ExecutionID) {
		return ErrInvalidAdmission
	}
	return nil
}

// PipelineRunCreator is an outbound seam only. Its caller must hold the sole
// send permit returned after committing SUBMITTING, then persist the observation.
// Display names and structurally valid requests never authorize repeating a POST
// or adopting a Run. This interface is not a leased worker or a product entry point.
type PipelineRunCreator interface {
	CreateRun(context.Context, PipelineCreateRequest) (PipelineSubmissionObservation, error)
}
