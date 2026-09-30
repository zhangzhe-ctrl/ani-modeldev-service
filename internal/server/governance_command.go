package server

import (
	"crypto/tls"
	"crypto/x509"
	"errors"

	"github.com/go-kratos/kratos/v3/middleware"
	kratosgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
)

// CommandTLS contains explicit deployment inputs. No environment certificate,
// endpoint or peer name is supplied by a default or by the RPC body.
type CommandTLS struct {
	Certificate tls.Certificate
	ClientCAs *x509.CertPool
	GovernanceDNSName string
}

// NewGovernanceCommandServer registers the command surface on a dedicated mTLS
// server. The initial RED candidate's handler is fail-closed and unimplemented;
// method/peer authorization must be added before acknowledging any command.
func NewGovernanceCommandServer(c *conf.Server_GRPC, security CommandTLS, command modeldevv1.ModelDevCommandServiceServer, middlewares ...middleware.Middleware) (*kratosgrpc.Server, error) {
	if c == nil || security.ClientCAs == nil || len(security.Certificate.Certificate) == 0 || security.Certificate.PrivateKey == nil || security.GovernanceDNSName == "" || command == nil {
		return nil, errors.New("explicit command TLS and handler configuration required")
	}
	s := kratosgrpc.NewServer(
		kratosgrpc.Network(c.Network), kratosgrpc.Address(c.Addr),
		kratosgrpc.Timeout(c.Timeout.AsDuration()),
		kratosgrpc.TLSConfig(&tls.Config{
			MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs: security.ClientCAs.Clone(), Certificates: []tls.Certificate{security.Certificate},
		}),
		kratosgrpc.Middleware(middlewares...), kratosgrpc.DisableReflection(),
	)
	modeldevv1.RegisterModelDevCommandServiceServer(s, command)
	return s, nil
}
