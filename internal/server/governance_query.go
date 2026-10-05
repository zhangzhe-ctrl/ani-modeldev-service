package server

import (
	"github.com/go-kratos/kratos/v3/middleware"
	kratosgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
)

func NewGovernanceQueryServer(c *conf.Server_GRPC, security CommandTLS, command modeldevv1.ModelDevCommandServiceServer, admission modeldevv1.ModelDevAdmissionServiceServer, query modeldevv1.ModelDevQueryServiceServer, middlewares ...middleware.Middleware) (*kratosgrpc.Server, error) {
	return newGovernanceServer(c, security, command, admission, query, GovernanceServices{}, middlewares...)
}

func NewGovernanceServicesServer(c *conf.Server_GRPC, security CommandTLS, command modeldevv1.ModelDevCommandServiceServer, admission modeldevv1.ModelDevAdmissionServiceServer, query modeldevv1.ModelDevQueryServiceServer, extra GovernanceServices, middlewares ...middleware.Middleware) (*kratosgrpc.Server, error) {
	return newGovernanceServer(c, security, command, admission, query, extra, middlewares...)
}
