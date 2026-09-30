package biz

import (
	"errors"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var ErrInputVerification = errors.New("INPUT_VERIFICATION_FAILED")

// VerifiedCSV combines actual immutable object bytes with the registered CPU
// input shape. It is an observation, not a persisted READY input or access grant.
type VerifiedCSV struct {
	VerifiedObject
	SchemaVersion string
	RowCount      uint32
	FeatureCount  uint32
}

// InputImport is a trusted request fixed before any remote validation. Tenant,
// actor and storage scope must come from the managed import authorization path.
// Neither a request nor its durable receipt establishes a READY input.
type InputImport struct {
	TenantID string
	RequestID string
	InputVersionID string
	Actor string
	RequestedAt time.Time
	Scope cpup01.StorageScope
	Object cpup01.FixedObjectRef
}

type InputState string

const InputStateValidating InputState = "VALIDATING"

type InputVersion struct {
	Import InputImport
	State InputState
}
