package biz

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidPipelineSubmitter    = errors.New("INVALID_PIPELINE_SUBMITTER_CONFIGURATION")
	ErrPipelineSubmissionUncertain = errors.New("PIPELINE_SUBMISSION_UNCERTAIN")
	ErrPipelineSubmissionNotSent = errors.New("PIPELINE_SUBMISSION_NOT_SENT")
)

// PipelineSubmitResult separates the last acknowledged durable dispatch from
// this call's transient observation. On a persistence error, Observation may
// retain a RunID which was not durably acknowledged; it is not a success receipt.
// A replay has no new observation and never carries another send permit.
type PipelineSubmitResult struct {
	Dispatch    PipelineDispatch
	Observation *PipelineSubmissionObservation
}

// PipelineSubmitter connects a committed reservation to at most one network
// call. It does not enumerate work, lease attempts, reconcile or grant authority.
type PipelineSubmitter struct {
	repository              PipelineDispatchRepository
	creator                 PipelineRunCreator
	observationWriteTimeout time.Duration
}

func NewPipelineSubmitter(repository PipelineDispatchRepository, creator PipelineRunCreator, observationWriteTimeout time.Duration) (*PipelineSubmitter, error) {
	if repository == nil || creator == nil || observationWriteTimeout <= 0 || observationWriteTimeout > 10*time.Second {
		return nil, ErrInvalidPipelineSubmitter
	}
	return &PipelineSubmitter{repository: repository, creator: creator, observationWriteTimeout: observationWriteTimeout}, nil
}

func (submitter *PipelineSubmitter) Submit(ctx context.Context, request PipelineDispatchRequest) (PipelineSubmitResult, error) {
	if ctx == nil {
		return PipelineSubmitResult{}, ErrInvalidAdmission
	}
	if err := ctx.Err(); err != nil {
		return PipelineSubmitResult{}, err
	}
	if submitter == nil || submitter.repository == nil || submitter.creator == nil {
		return PipelineSubmitResult{}, ErrPersistence
	}
	reservation, err := submitter.repository.Reserve(ctx, request)
	if err != nil {
		return PipelineSubmitResult{}, err
	}
	result := PipelineSubmitResult{Dispatch: reservation.Dispatch}
	if reservation.SendPermit == nil {
		// Even SUBMITTING after a caller restart is only a durable fact. Do not
		// reconstruct permission, regenerate credentials or repeat the POST.
		return result, nil
	}
	permit := *reservation.SendPermit
	input := PipelineCreateRequest{Admission: request.Admission, Plan: reservation.Dispatch.Plan, Permit: permit}
	if reservation.Dispatch.State != PipelineDispatchSubmitting || reservation.Dispatch.UncertainAt != nil || reservation.Dispatch.NotSentAt != nil || len(reservation.Dispatch.ConfirmedRuns) != 0 ||
		!strings.EqualFold(permit.AttemptID, reservation.Dispatch.AttemptID) || permit.PlanHash != reservation.Dispatch.PlanHash || input.Plan.Owner != request.Owner || input.Validate() != nil {
		return result, ErrPersistence
	}
	// Reserve has committed and released its transaction. Only this original
	// permit reaches the actual network call, using the returned frozen plan.
	observation, callErr := submitter.creator.CreateRun(ctx, input)
	result.Observation = &observation
	switch observation.State {
	case PipelineSubmissionConfirmed:
		if callErr != nil || !validAdmissionID(observation.RunID) {
			return result, ErrPersistence
		}
	case PipelineSubmissionUncertain, PipelineSubmissionNotSent:
		if callErr == nil || observation.RunID != "" {
			return result, ErrPersistence
		}
	default:
		// Retain unknown/error observations without inventing a durable result.
		return result, ErrPersistence
	}
	// A canceled caller must not erase a response from the original attempt.
	// One finite budget covers clock sampling and one persistence transaction;
	// no background worker, retry loop or second POST is created.
	writeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), submitter.observationWriteTimeout)
	defer cancel()
	observedAt, err := submitter.repository.SubmissionObservationTime(writeContext, permit)
	if err != nil {
		return result, ErrPersistence
	}
	if observation.State == PipelineSubmissionConfirmed {
		receipt, err := submitter.repository.RecordSubmissionConfirmed(writeContext, permit, observation, observedAt)
		if err != nil {
			return result, ErrPersistence
		}
		result.Dispatch = receipt.Dispatch
		return result, nil
	}
	if observation.State == PipelineSubmissionNotSent {
		dispatch, err := submitter.repository.MarkSubmissionNotSent(writeContext, permit, observedAt)
		if err != nil {
			return result, ErrPersistence
		}
		result.Dispatch = dispatch
		return result, ErrPipelineSubmissionNotSent
	}
	dispatch, err := submitter.repository.MarkSubmissionUncertain(writeContext, permit, observedAt)
	if err != nil {
		return result, ErrPersistence
	}
	result.Dispatch = dispatch
	return result, ErrPipelineSubmissionUncertain
}
