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
	if err := ctx.Err(); err != nil {
		return InputVersion{}, err
	}
	if err := request.Validate(); err != nil {
		return InputVersion{}, err
	}
	if importer == nil || importer.repository == nil || importer.verifier == nil {
		return InputVersion{}, ErrPersistence
	}
	frozen, err := importer.repository.FreezeImport(ctx, request)
	if err != nil {
		return InputVersion{}, err
	}
	if frozen.State == InputStateReady {
		return frozen, nil
	}
	if frozen.State != InputStateValidating {
		return InputVersion{}, ErrPersistence
	}
	// The receipt has committed before this network call. Use its original
	// version and approved scope, including when resuming after a process exit.
	verified, err := importer.verifier.VerifyCSV(ctx, frozen.Import.Scope, frozen.Import.Object)
	if err != nil {
		return frozen, err
	}
	if err := verified.ValidateFor(frozen.Import); err != nil {
		return frozen, err
	}
	return importer.repository.RecordVerifiedCSV(ctx, frozen.Import, verified)
}
