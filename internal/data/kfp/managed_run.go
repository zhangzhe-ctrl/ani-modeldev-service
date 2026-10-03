package kfp

import (
	"context"
	"errors"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// VerifyManagedRun reads KFP's own Run/task association, independently of
// caller claims and Kubernetes labels. It never creates or retries a Run.
func (client *Client) VerifyManagedRun(ctx context.Context, plan biz.PipelineDispatchPlan, association biz.ManagedStepAssociation, taskName string) error {
	return errors.New("MANAGED_RUN_NOT_IMPLEMENTED")
}
