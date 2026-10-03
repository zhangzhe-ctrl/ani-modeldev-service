// Package component implements one invocation of one managed KFP component.
// KFP chooses the next step; this package never advances a Go workflow or
// creates training resources directly.
package component

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"k8s.io/client-go/dynamic"
)

var ErrConfiguration = errors.New("MANAGED_COMPONENT_INVALID_CONFIGURATION")
var ErrTrainingFailed = errors.New("MANAGED_COMPONENT_TRAINING_FAILED")
var ErrTrainingCanceled = errors.New("MANAGED_COMPONENT_TRAINING_CANCELED")
var ErrNeedsReview = errors.New("MANAGED_COMPONENT_CLOSE_NEEDS_REVIEW")

var bearerPattern = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

type Config struct {
	TenantID           string
	Context            *modeldevv1.StepContext
	TokenFile          string
	WorkspaceDirectory string
	PVCName            string
	InventoryFile      string
	CandidateFile      string
	TaskID             string
	PollInterval       time.Duration
}

type Runner struct {
	config Config
	client modeldevv1.ModelDevStepServiceClient
	kube   dynamic.Interface
	store  *s3.Client
}

func New(config Config, client modeldevv1.ModelDevStepServiceClient, kube dynamic.Interface, store *s3.Client) (*Runner, error) {
	if client == nil || config.Context == nil || config.Context.Identity == nil || config.Context.Association == nil || config.TenantID == "" || config.TokenFile == "" {
		return nil, ErrConfiguration
	}
	identifiers := []string{config.TenantID, config.Context.Identity.ExecutionId, config.Context.Association.KfpRunId, config.Context.Association.NamespaceUid, config.Context.Association.PodUid}
	if config.Context.Identity.OperationId != "" {
		identifiers = append(identifiers, config.Context.Identity.OperationId)
	}
	for _, value := range identifiers {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil || id.String() != value {
			return nil, ErrConfiguration
		}
	}
	if !filepath.IsAbs(config.TokenFile) || config.Context.Association.NamespaceName == "" || config.Context.Association.WorkflowName == "" || config.Context.Association.WorkflowUid == "" || config.Context.Association.PodName == "" || len(config.Context.Identity.ExecutionSpecHash) != 64 {
		return nil, ErrConfiguration
	}
	if _, err := hex.DecodeString(config.Context.Identity.ExecutionSpecHash); err != nil || strings.ToLower(config.Context.Identity.ExecutionSpecHash) != config.Context.Identity.ExecutionSpecHash {
		return nil, ErrConfiguration
	}
	if config.PollInterval == 0 {
		config.PollInterval = time.Second
	}
	if config.PollInterval < time.Millisecond || config.PollInterval > 30*time.Second {
		return nil, ErrConfiguration
	}
	config.Context = proto.Clone(config.Context).(*modeldevv1.StepContext)
	return &Runner{config: config, client: client, kube: kube, store: store}, nil
}

func (runner *Runner) Run(ctx context.Context, step string) error {
	if runner == nil || ctx == nil {
		return ErrConfiguration
	}
	wanted, ok := map[string]modeldevv1.PipelineStep{"prepare": modeldevv1.PipelineStep_PIPELINE_STEP_PREPARE, "train-wait": modeldevv1.PipelineStep_PIPELINE_STEP_TRAIN_WAIT, "collect": modeldevv1.PipelineStep_PIPELINE_STEP_COLLECT, "publish": modeldevv1.PipelineStep_PIPELINE_STEP_PUBLISH, "close": modeldevv1.PipelineStep_PIPELINE_STEP_CLOSE}[step]
	if !ok || runner.config.Context.Step != wanted {
		return ErrConfiguration
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	switch step {
	case "prepare":
		return runner.prepare(ctx)
	case "train-wait":
		return runner.trainWait(ctx)
	case "collect":
		return runner.collect(ctx)
	case "publish":
		return runner.publish(ctx)
	case "close":
		return runner.close(ctx)
	default:
		return ErrConfiguration
	}
}

func (runner *Runner) callContext(ctx context.Context) (context.Context, error) {
	data, err := readToken(runner.config.TokenFile)
	if err != nil {
		return nil, ErrConfiguration
	}
	token := strings.TrimSpace(string(data))
	if !bearerPattern.MatchString(token) {
		return nil, ErrConfiguration
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token, "x-ani-tenant-id", runner.config.TenantID)), nil
}

func (runner *Runner) trainWait(ctx context.Context) error {
	if runner.config.Context.Identity.OperationId == "" {
		if _, _, err := runner.configuration(ctx); err != nil {
			return err
		}
	}
	call, err := runner.callContext(ctx)
	if err != nil {
		return err
	}
	ensured, err := runner.client.EnsureTraining(call, &modeldevv1.EnsureTrainingRequest{Context: runner.config.Context})
	if err != nil && !retryObservation(err) {
		return err
	}
	if err == nil {
		done, err := runner.trainingResult(ensured.GetStatus())
		if done || err != nil {
			return err
		}
	}
	for {
		if err := runner.wait(ctx); err != nil {
			return err
		}
		call, err := runner.callContext(ctx)
		if err != nil {
			return err
		}
		observed, err := runner.client.GetTrainingStatus(call, &modeldevv1.GetTrainingStatusRequest{Context: runner.config.Context})
		if err != nil {
			if retryObservation(err) {
				continue
			}
			return err
		}
		done, err := runner.trainingResult(observed.GetStatus())
		if done || err != nil {
			return err
		}
	}
}

func (runner *Runner) trainingResult(training *modeldevv1.TrainingStatus) (bool, error) {
	if training == nil || !proto.Equal(training.Identity, runner.config.Context.Identity) || training.States == nil {
		return false, ErrConfiguration
	}
	switch training.States.ComputeState {
	case modeldevv1.ComputeState_COMPUTE_STATE_SUCCEEDED:
		return true, nil
	case modeldevv1.ComputeState_COMPUTE_STATE_FAILED:
		return true, ErrTrainingFailed
	case modeldevv1.ComputeState_COMPUTE_STATE_CANCELED:
		return true, ErrTrainingCanceled
	case modeldevv1.ComputeState_COMPUTE_STATE_UNSPECIFIED:
		return false, ErrConfiguration
	default:
		return false, nil
	}
}

func (runner *Runner) wait(ctx context.Context) error {
	timer := time.NewTimer(runner.config.PollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryObservation(err error) bool {
	code := status.Code(err)
	return code == codes.Unavailable || code == codes.FailedPrecondition
}

func (runner *Runner) reportID(kind string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(runner.config.TenantID+"\x00"+runner.config.Context.Identity.ExecutionId+"\x00"+runner.config.Context.Identity.ExecutionSpecHash+"\x00"+kind)).String()
}
