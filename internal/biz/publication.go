package biz

import (
	"errors"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var ErrObjectVerification = errors.New("OBJECT_VERIFICATION_FAILED")

// VerifiedObject is a byte-verification observation. It is not a durable
// publication, uploader completion proof, or permission to issue a download.
type VerifiedObject struct {
	Object cpup01.FixedObjectRef
	VerifiedAt time.Time
}
