package biz

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidPipelineSubmitter = errors.New("INVALID_PIPELINE_SUBMITTER_CONFIGURATION")
	ErrPipelineSubmissionUncertain = errors.New("PIPELINE_SUBMISSION_UNCERTAIN")
)

// PipelineSubmitResult separates the last acknowledged durable dispatch from
// this call's transient observation. On a persistence error, Observation may
// retain a RunID which was not durably acknowledged; it is not a success receipt.
// A replay has no new observation and never carries another send permit.
type PipelineSubmitResult struct {
	Dispatch PipelineDispatch
	Observation *PipelineSubmissionObservation
}

// PipelineSubmitter connects a committed reservation to at most one network
// call. It does not enumerate work, lease attempts, reconcile or grant authority.
type PipelineSubmitter struct {
	repository PipelineDispatchRepository
	creator PipelineRunCreator
	observationWriteTimeout time.Duration
}

func NewPipelineSubmitter(repository PipelineDispatchRepository, creator PipelineRunCreator, observationWriteTimeout time.Duration) (*PipelineSubmitter, error) {
	if repository == nil || creator == nil || observationWriteTimeout <= 0 || observationWriteTimeout > 10*time.Second {
		return nil, ErrInvalidPipelineSubmitter
	}
	return &PipelineSubmitter{repository: repository, creator: creator, observationWriteTimeout: observationWriteTimeout}, nil
}

func (submitter *PipelineSubmitter) Submit(ctx context.Context, request PipelineDispatchRequest) (PipelineSubmitResult, error) {
	// Explicit first-behavior stub: no reservation, credentials or network work
	// has happened. Real PG/TLS integration must expose this missing behavior.
	return PipelineSubmitResult{}, ErrPersistence
}
