// Package stepidentity verifies a managed callback against the current
// Kubernetes TokenReview and resource APIs. KFP Run membership is verified by
// a separate KFP adapter before the use case binds authority.
package stepidentity

import (
	"context"
	"errors"
	"strings"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"k8s.io/client-go/dynamic"
)

var (
	ErrInvalidConfig = errors.New("MANAGED_STEP_IDENTITY_INVALID_CONFIGURATION")
	ErrUnverified = errors.New("MANAGED_STEP_IDENTITY_UNVERIFIED")
)

type Verifier struct {
	client dynamic.Interface
	audience string
}

func New(client dynamic.Interface, audience string) (*Verifier, error) {
	if client == nil || audience == "" || strings.TrimSpace(audience) != audience {
		return nil, ErrInvalidConfig
	}
	return &Verifier{client: client, audience: audience}, nil
}

func (verifier *Verifier) Verify(context.Context, string, biz.PipelineDispatchPlan, biz.ManagedStepAssociation) error {
	return errors.New("MANAGED_STEP_IDENTITY_NOT_IMPLEMENTED")
}
