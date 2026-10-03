// Package component implements one invocation of one managed KFP component.
// KFP chooses the next step; this package never advances a Go workflow or
// creates training resources directly.
package component

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"k8s.io/client-go/dynamic"
)

var ErrConfiguration = errors.New("MANAGED_COMPONENT_INVALID_CONFIGURATION")

type Config struct {
	TenantID string
	Context *modeldevv1.StepContext
	TokenFile string
	WorkspaceDirectory string
	PVCName string
	InventoryFile string
	CandidateFile string
	TaskID string
	PollInterval time.Duration
}

type Runner struct {
	config Config
	client modeldevv1.ModelDevStepServiceClient
	kube dynamic.Interface
	store *s3.Client
}

func New(config Config, client modeldevv1.ModelDevStepServiceClient, kube dynamic.Interface, store *s3.Client) (*Runner,error) {
	if client == nil || config.Context == nil || config.Context.Identity == nil || config.Context.Association == nil || config.TenantID == "" || config.TokenFile == "" {
		return nil,ErrConfiguration
	}
	return &Runner{config:config,client:client,kube:kube,store:store},nil
}

func (runner *Runner) Run(context.Context,string) error {
	return errors.New("MANAGED_COMPONENT_NOT_IMPLEMENTED")
}
