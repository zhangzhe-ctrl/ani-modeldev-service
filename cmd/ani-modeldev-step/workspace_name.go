package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/component"
)

// This component computes a retained PVC name for KFP's official CreatePVC.
// It has no Kubernetes client, credentials, or authority to create resources.
func writeWorkspaceName(args []string) error {
	flags := flag.NewFlagSet("workspace-name", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	execution := flags.String("execution-id", "", "")
	output := flags.String("output", "", "")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return component.ErrConfiguration
	}
	id, err := uuid.Parse(*execution)
	if err != nil || id == uuid.Nil || id.String() != *execution || !filepath.IsAbs(*output) || filepath.Clean(*output) != *output {
		return component.ErrConfiguration
	}
	file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return component.ErrConfiguration
	}
	defer file.Close()
	name := "ani-kfp-workspace-" + id.String()
	if _, err = file.WriteString(name); err != nil {
		return component.ErrConfiguration
	}
	return file.Close()
}
