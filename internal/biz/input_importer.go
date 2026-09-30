package biz

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

// ManagedInputRepository commits the immutable request and its verified state.
type ManagedInputRepository interface {
	FreezeImport(context.Context, InputImport) (InputVersion, error)
	RecordVerifiedCSV(context.Context, InputImport, VerifiedCSV) (InputVersion, error)
}

type CSVVerifier interface {
	VerifyCSV(context.Context, cpup01.StorageScope, cpup01.FixedObjectRef) (VerifiedCSV, error)
}

// InputImporter runs only after the managed entry point authenticates the
// current administrator and authorizes the target tenant and source scope.
// The request carries an explicit fixed version, never an arbitrary endpoint.
type InputImporter struct {
	repository ManagedInputRepository
	verifier CSVVerifier
}

func NewInputImporter(repository ManagedInputRepository, verifier CSVVerifier) *InputImporter {
	return &InputImporter{repository: repository, verifier: verifier}
}

func (importer *InputImporter) ImportCSV(ctx context.Context, request InputImport) (InputVersion, error) {
	return InputVersion{}, ErrPersistence
}
