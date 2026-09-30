package main

import (
	"io"
	"path/filepath"
	"testing"
	"time"

	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestConfiguredCommandDoesNotFallBackWhenConnectionsAreMissing(t *testing.T) {
	config := commandAppConfig(t)
	root := t.TempDir()
	config.Command = &conf.GovernanceCommand{
		DatabaseUrlFile: filepath.Join(root, "missing-database"), ClientCaFile: filepath.Join(root, "missing-ca.crt"),
		CertificateFile: filepath.Join(root, "missing-tls.crt"), PrivateKeyFile: filepath.Join(root, "missing-tls.key"), GovernanceDnsName: "ani-governance",
	}
	if app, err := buildApp(config, newRuntimeLogger(io.Discard)); err == nil || app != nil {
		t.Fatal("configured command must fail startup when its actual connection materials are missing")
	}
}

func commandAppConfig(t *testing.T) *conf.Bootstrap {
	t.Helper()
	grpcAddress, adminAddress := reserveAddress(t), reserveAddress(t)
	for grpcAddress == adminAddress { adminAddress = reserveAddress(t) }
	return &conf.Bootstrap{Server: &conf.Server{
		Grpc: &conf.Server_GRPC{Network: "tcp", Addr: grpcAddress, Timeout: durationpb.New(3*time.Second)},
		Admin: &conf.Server_Admin{Network: "tcp", Addr: adminAddress, Timeout: durationpb.New(time.Second)},
		ShutdownTimeout: durationpb.New(5*time.Second),
	}}
}
