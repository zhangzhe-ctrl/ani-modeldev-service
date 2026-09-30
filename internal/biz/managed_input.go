package biz

import "errors"

var ErrInputVerification = errors.New("INPUT_VERIFICATION_FAILED")

// VerifiedCSV combines actual immutable object bytes with the registered CPU
// input shape. It is an observation, not a persisted READY input or access grant.
type VerifiedCSV struct {
	VerifiedObject
	SchemaVersion string
	RowCount      uint32
	FeatureCount  uint32
}
