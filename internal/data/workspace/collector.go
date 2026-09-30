// Package workspace implements access to a managed execution workspace.
package workspace

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// Collect reads the registered CPU outputs under a trusted mounted training
// directory. Its caller owns authentication, workspace UID binding and writer
// closure; this adapter does not accept user-selected host paths.
func Collect(ctx context.Context, directory string, execution biz.Execution) (biz.CollectedOutput, error) {
	return biz.CollectedOutput{}, biz.ErrInvalidWorkspaceOutput
}
