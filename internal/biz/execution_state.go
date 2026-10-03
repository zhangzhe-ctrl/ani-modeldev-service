package biz

import "errors"

var ErrInvalidExecutionStateFacts = errors.New("INVALID_EXECUTION_STATE_FACTS")

type ComputeState string

const (
	ComputeStateAccepted            ComputeState = "ACCEPTED"
	ComputeStateSubmitting          ComputeState = "SUBMITTING"
	ComputeStateSubmissionNotSent   ComputeState = "SUBMISSION_NOT_SENT"
	ComputeStateSubmissionUncertain ComputeState = "SUBMISSION_UNCERTAIN"
	ComputeStateSubmissionConfirmed ComputeState = "SUBMISSION_CONFIRMED"
)

type DeliveryState string

const DeliveryStatePending DeliveryState = "PENDING"

type ResourceState string

const ResourceStateNotApplicable ResourceState = "NOT_APPLICABLE"

// ExecutionStates reports independent axes of observed execution facts. It
// grants neither a creation permit nor authority to an observed Pipeline Run.
type ExecutionStates struct {
	Compute  ComputeState
	Delivery DeliveryState
	Resource ResourceState
	Close    CloseState
}

// ExecutionStateFacts contains validated facts read with an existing Admission
// in one transaction. Nil means no persisted dispatch or close respectively;
// a present value must be a supported persisted state, never a default value.
type ExecutionStateFacts struct {
	Submission *PipelineDispatchState
	Close      *CloseState
}

// Project describes the current CPU admission/submission fact set. Publishing
// and runtime facts must extend this input when their writers are introduced.
// A close affects only its own axis; submission observations establish neither
// queued computation nor a training outcome.
func (facts ExecutionStateFacts) Project() (ExecutionStates, error) {
	states := ExecutionStates{
		Compute:  ComputeStateAccepted,
		Delivery: DeliveryStatePending,
		Resource: ResourceStateNotApplicable,
		Close:    CloseStateOpen,
	}
	if facts.Submission != nil {
		switch *facts.Submission {
		case PipelineDispatchSubmitting:
			states.Compute = ComputeStateSubmitting
		case PipelineDispatchNotSent:
			states.Compute = ComputeStateSubmissionNotSent
		case PipelineDispatchUncertain:
			states.Compute = ComputeStateSubmissionUncertain
		case PipelineDispatchConfirmed:
			states.Compute = ComputeStateSubmissionConfirmed
		default:
			return ExecutionStates{}, ErrInvalidExecutionStateFacts
		}
	}
	if facts.Close != nil {
		if *facts.Close != CloseStateClosing {
			return ExecutionStates{}, ErrInvalidExecutionStateFacts
		}
		states.Close = CloseStateClosing
	}
	return states, nil
}
