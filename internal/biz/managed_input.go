package biz

import (
	"errors"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var ErrInputVerification = errors.New("INPUT_VERIFICATION_FAILED")

var (
	ErrInvalidInput = errors.New("INVALID_INPUT")
	ErrInputConflict = errors.New("INPUT_IMPORT_CONFLICT")
	ErrInputNotFound = errors.New("INPUT_VERSION_NOT_FOUND")
)

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
	TenantID       string
	RequestID      string
	InputVersionID string
	Actor          string
	RequestedAt    time.Time
	Scope          cpup01.StorageScope
	Object         cpup01.FixedObjectRef
}

type InputState string

const InputStateValidating InputState = "VALIDATING"

type InputVersion struct {
	Import InputImport
	State  InputState
}

// Validate checks the immutable request's structure, not source permissions or
// remote content. An explicit object version is required before validation starts.
func (request InputImport) Validate() error {
	if !validAdmissionID(request.TenantID) || !validAdmissionID(request.RequestID) || !validAdmissionID(request.InputVersionID) || !validAuditActor(request.Actor) {
		return ErrInvalidInput
	}
	when := request.RequestedAt.UTC()
	if when.IsZero() || when.Year() < 1 || when.Year() > 9999 || when.Nanosecond()%1000 != 0 {
		return ErrInvalidInput
	}
	object, scope := request.Object, request.Scope
	if !validInputText(scope.StorageConnectionID) || !validInputText(scope.Bucket) || object.StorageConnectionID != scope.StorageConnectionID || object.Bucket != scope.Bucket || !validInputKey(scope.ApprovedPrefix) || !validInputKey(object.Key) || !strings.HasPrefix(object.Key, scope.ApprovedPrefix+"/") {
		return ErrInvalidInput
	}
	if object.VersionID == nil || object.ImmutableCopy != nil || !validInputText(*object.VersionID) || strings.EqualFold(*object.VersionID, "null") || object.SizeBytes <= 0 || object.SizeBytes > 32*1024*1024 || !closeSpecHashPattern.MatchString(object.SHA256) {
		return ErrInvalidInput
	}
	return nil
}

func validInputText(value string) bool {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) { return false }
	}
	return true
}

func validInputKey(value string) bool {
	return validInputText(value) && len(value) <= 1024 && value != "." && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && path.Clean(value) == value
}
