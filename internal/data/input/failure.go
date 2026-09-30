package input

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// RecordValidationFailure records a trusted verifier's finite observation.
// Source unavailability remains retryable against the same fixed object;
// rejected content is terminal and must never be promoted by a late verifier.
func (r *Repository) RecordValidationFailure(context.Context, biz.InputImport, biz.InputValidationFailure) (biz.InputVersion, error) {
	return biz.InputVersion{}, biz.ErrPersistence
}
