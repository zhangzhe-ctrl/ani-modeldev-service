package kfp

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// StopManagedRun is the trusted owner close boundary for a bound Run.
func (client *Client) StopManagedRun(ctx context.Context, plan biz.PipelineDispatchPlan, authority biz.RunAuthorityCandidate) error {
	return biz.ErrRuntimeNotReady
}
