// Package kfp owns the narrow outbound KFP CreateRun boundary.
package kfp

import (
	"context"
	"crypto/x509"
	"errors"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

var ErrNotImplemented = errors.New("KFP_CREATE_RUN_NOT_IMPLEMENTED")

// Config is supplied by the owner, never by a browser request. No endpoint or
// artifact-root default is available. PipelineRoot is distinct from publication
// storage; freezing it into the execution contract is a prerequisite to wiring.
type Config struct {
	ConnectionRef string
	Endpoint string
	PipelineRoot string
	RootCAs *x509.CertPool
	Timeout time.Duration
}

// TokenProvider must authenticate the control workload and resolve the trusted
// tenant/binding identity, including TenantProxyIdentity, into a short-lived
// bearer credential. Its implementation must not forward browser headers or
// treat the admission's audit actor as a permanent authorization grant.
type TokenProvider interface {
	BearerToken(context.Context, string, cpup01.EnvironmentBindingSnapshot) (string, error)
}

// Client is not wired into a product entry point. The initial explicit stub
// allows the first fixed-SHA Fedora run to fail on behavior, not compilation.
type Client struct{}

func New(_ Config, _ TokenProvider) (*Client, error) {
	return &Client{}, nil
}

func (*Client) CreateRun(context.Context, biz.Admission) (biz.PipelineSubmissionObservation, error) {
	return biz.PipelineSubmissionObservation{}, ErrNotImplemented
}

var _ biz.PipelineRunCreator = (*Client)(nil)
