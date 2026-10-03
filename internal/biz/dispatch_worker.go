package biz

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var ErrInvalidDispatchWorker = errors.New("INVALID_DISPATCH_WORKER_CONFIGURATION")

// PipelineDispatchBinding is trusted service-owner configuration, not request
// data. Only admissions already frozen to this exact tenant/environment may use
// its explicitly supplied PipelineRoot and owner revision.
type PipelineDispatchBinding struct {
	TenantID    string                            `json:"tenant_id"`
	Environment cpup01.EnvironmentBindingSnapshot `json:"environment"`
	Owner       PipelineOwnerConfiguration        `json:"owner"`
}

var dispatchDNSLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func (binding PipelineDispatchBinding) Validate() error {
	environment := binding.Environment
	for _, id := range []string{binding.TenantID, environment.BindingID, environment.NamespaceUID, environment.ExperimentID} {
		if !validAdmissionID(id) || strings.ToLower(id) != id {
			return ErrInvalidDispatchWorker
		}
	}
	if !closeSpecHashPattern.MatchString(environment.BindingDigest) || !closeSpecHashPattern.MatchString(binding.Owner.RevisionSHA256) || !ValidPipelineRoot(binding.Owner.PipelineRoot) {
		return ErrInvalidDispatchWorker
	}
	for _, reference := range []string{environment.ClusterID, environment.KFPConnectionRef, environment.Identities.ModeldevControlIdentityRef, environment.Identities.TenantProxyIdentity, binding.Owner.Reference} {
		if !pipelineOwnerReferencePattern.MatchString(reference) || strings.Contains(reference, "://") {
			return ErrInvalidDispatchWorker
		}
	}
	if !validDispatchDNS(environment.NamespaceName, true) {
		return ErrInvalidDispatchWorker
	}
	accounts := map[string]bool{}
	for _, account := range []string{environment.Identities.KFPStepServiceAccount, environment.Identities.TrainerServiceAccount, environment.Identities.VerifierServiceAccount} {
		if !validDispatchDNS(account, false) || accounts[account] {
			return ErrInvalidDispatchWorker
		}
		accounts[account] = true
	}
	return nil
}
func validDispatchDNS(value string, labelOnly bool) bool {
	if value == "" || len(value) > 253 || labelOnly && len(value) > 63 {
		return false
	}
	labels := strings.Split(value, ".")
	if labelOnly && len(labels) != 1 {
		return false
	}
	for _, label := range labels {
		if len(label) > 63 || !dispatchDNSLabel.MatchString(label) {
			return false
		}
	}
	return true
}

type PendingAdmissionRepository interface {
	ListPendingAdmissions(context.Context, PipelineDispatchBinding, int) ([]Admission, error)
}

// DispatchWorker only discovers persisted admissions. Reserve remains the sole
// creator of a sending permit, so independent workers need no second lease or
// queue. Existing attempts, including uncertain/not-sent attempts, are excluded
// from scanning and can never be implicitly resubmitted by this worker.
type DispatchWorker struct {
	repository PendingAdmissionRepository
	submitter  *PipelineSubmitter
	binding    PipelineDispatchBinding
	batchSize  int
	interval   time.Duration
}

func NewDispatchWorker(repository PendingAdmissionRepository, submitter *PipelineSubmitter, binding PipelineDispatchBinding, batchSize int, interval time.Duration) (*DispatchWorker, error) {
	if repository == nil || submitter == nil || binding.Validate() != nil || batchSize < 1 || batchSize > 100 || interval < 100*time.Millisecond || interval > time.Minute {
		return nil, ErrInvalidDispatchWorker
	}
	return &DispatchWorker{repository: repository, submitter: submitter, binding: binding, batchSize: batchSize, interval: interval}, nil
}

// DispatchOnce returns the number of eligible admissions handled by this call,
// not a count of outbound calls or successful Runs. Only the submitter receipt
// and persisted observations establish the outcome of a reservation.
func (worker *DispatchWorker) DispatchOnce(ctx context.Context) (int, error) {
	if ctx == nil {
		return 0, ErrInvalidAdmission
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if worker == nil || worker.repository == nil || worker.submitter == nil {
		return 0, ErrInvalidDispatchWorker
	}
	admissions, err := worker.repository.ListPendingAdmissions(ctx, worker.binding, worker.batchSize)
	if err != nil {
		return 0, err
	}
	if len(admissions) > worker.batchSize {
		return 0, ErrPersistence
	}
	handled := 0
	for _, admission := range admissions {
		if admission.TenantID != worker.binding.TenantID || admission.Snapshot.Environment != worker.binding.Environment {
			return handled, ErrAdmissionConflict
		}
		if _, _, err := admission.CanonicalPayloads(); err != nil {
			return handled, ErrPersistence
		}
		_, err = worker.submitter.Submit(ctx, PipelineDispatchRequest{Admission: admission, Owner: worker.binding.Owner})
		switch {
		case err == nil, errors.Is(err, ErrPipelineSubmissionUncertain), errors.Is(err, ErrPipelineSubmissionNotSent):
			handled++
		case errors.Is(err, ErrPipelineDispatchBlocked):
			// A close or deadline may win after enumeration. Reserve rejected it
			// under the shared identity lock; no creation is attempted here.
		default:
			return handled, err
		}
	}
	return handled, nil
}

// Run is an application-owned blocking loop. Its caller owns cancellation and
// waits for it before closing database/client dependencies. It does not spawn a
// detached task, retry a reserved attempt, or advance managed KFP steps.
func (worker *DispatchWorker) Run(ctx context.Context) error {
	if ctx == nil || worker == nil || worker.interval <= 0 {
		return ErrInvalidDispatchWorker
	}
	timer := time.NewTicker(worker.interval)
	defer timer.Stop()
	for {
		if _, err := worker.DispatchOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
	}
}
