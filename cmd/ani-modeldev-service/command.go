package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v3/middleware"
	kratosgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/jackc/pgx/v5/pgxpool"
	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/server"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/service"
)

// Command assembly consumes mounted references only. It does not run schema
// migrations, create roles, enable readiness, or fall back to another listener.
func buildCommandServer(listener *conf.Server_GRPC, config *conf.GovernanceCommand, middlewares ...middleware.Middleware) (*kratosgrpc.Server, func(), error) {
	failed := func(message string) (*kratosgrpc.Server, func(), error) { return nil, nil, errors.New(message) }
	ca, err := readCommandMaterial(config.ClientCaFile, 1<<20)
	if err != nil {
		return failed("command CA material unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return failed("command CA material invalid")
	}
	certificatePEM, err := readCommandMaterial(config.CertificateFile, 1<<20)
	if err != nil {
		return failed("command server certificate unavailable")
	}
	privateKeyPEM, err := readCommandMaterial(config.PrivateKeyFile, 1<<20)
	if err != nil {
		return failed("command server key unavailable")
	}
	certificate, err := tls.X509KeyPair(certificatePEM, privateKeyPEM)
	if err != nil {
		return failed("command server certificate pair invalid")
	}
	databaseBytes, err := readCommandMaterial(config.DatabaseUrlFile, 16<<10)
	if err != nil {
		return failed("command database reference unavailable")
	}
	connection := strings.TrimSpace(string(databaseBytes))
	if connection == "" {
		return failed("command database configuration invalid")
	}
	poolConfig, err := pgxpool.ParseConfig(connection)
	if err != nil {
		return failed("command database configuration invalid")
	}
	poolConfig.MaxConns = 4
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return failed("command database unavailable")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return failed("command database unavailable")
	}
	command := service.NewCommand(execution.New(pool))
	s, err := server.NewGovernanceCommandServer(listener, server.CommandTLS{Certificate: certificate, ClientCAs: roots, GovernanceDNSName: config.GovernanceDnsName}, command, nil, middlewares...)
	if err != nil {
		pool.Close()
		return failed("command listener configuration invalid")
	}
	return s, pool.Close, nil
}

func readCommandMaterial(path string, maximum int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		return nil, errors.New("command material unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("command material unavailable")
	}
	defer file.Close()
	value, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || len(value) == 0 || int64(len(value)) > maximum {
		return nil, errors.New("command material unavailable")
	}
	return value, nil
}
