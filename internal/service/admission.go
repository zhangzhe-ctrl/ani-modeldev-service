package service

import (
	"context"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Admission adapts authenticated Governance requests to read-only candidate
// resolution. It does not accept executions or emit durable command receipts.
type Admission struct {
	modeldevv1.UnimplementedModelDevAdmissionServiceServer
	resolver *biz.AdmissionResolver
}

func NewAdmission(resolver *biz.AdmissionResolver) *Admission {
	return &Admission{resolver: resolver}
}

func (*Admission) ResolveAdmission(context.Context, *modeldevv1.ResolveAdmissionRequest) (*modeldevv1.ResolveAdmissionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "admission resolution not implemented")
}
