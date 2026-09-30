// Package trainer owns ModelDev's outbound Kubeflow Trainer operations.
package trainer

import (
	"context"
	"errors"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"k8s.io/client-go/dynamic"
)

var ErrNotImplemented = errors.New("CPU06_NOT_IMPLEMENTED")

// Adapter uses the official Kubernetes client supplied by the composition root.
// No product transport is connected to this initial test-first slice.
type Adapter struct {
	client dynamic.Interface
}

func New(client dynamic.Interface) *Adapter {
	return &Adapter{client: client}
}

func (a *Adapter) ObserveTrainJob(context.Context, biz.TrainJobBinding) (biz.TrainJobObservation, error) {
	return biz.TrainJobObservation{}, ErrNotImplemented
}
