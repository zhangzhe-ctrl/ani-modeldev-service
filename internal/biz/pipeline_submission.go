package biz

import "context"

// PipelineSubmissionState describes an outbound call observation, not a
// persisted compute state, an authoritative Run, or a training creation permit.
type PipelineSubmissionState string

const (
	PipelineSubmissionNotSent PipelineSubmissionState = "NOT_SENT"
	PipelineSubmissionUncertain PipelineSubmissionState = "UNCERTAIN"
	PipelineSubmissionConfirmed PipelineSubmissionState = "CONFIRMED"
)

type PipelineSubmissionObservation struct {
	State PipelineSubmissionState
	RunID string
}

// PipelineRunCreator is an outbound seam only. Before calling it, a future
// leased worker must durably record SUBMITTING under the shared identity/close
// fence. The current admission repository does not yet implement that worker.
// Callers must persist the observation and reconcile uncertain calls; display
// names do not authorize repeating CreateRun or adopting a second Run.
type PipelineRunCreator interface {
	CreateRun(context.Context, Admission) (PipelineSubmissionObservation, error)
}
