package main

import (
	"errors"
	"log/slog"

	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
)

func buildRuntimeApp(config *conf.Bootstrap, logger *slog.Logger) (*application, error) {
	return nil, errors.New("RUNTIME_ASSEMBLY_NOT_IMPLEMENTED")
}
