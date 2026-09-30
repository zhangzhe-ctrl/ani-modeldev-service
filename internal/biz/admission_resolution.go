package biz

import (
	"context"
	"errors"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

// AdmissionReleaseSelection is one explicit Governance pointer observation.
// ModelDev must preserve its generation and never choose a current Release.
type AdmissionReleaseSelection struct {
	ReleaseID         string
	ReleaseDigest     string
	BindingGeneration uint64
}

// AdmissionResolutionRequest follows current authorization and an original-key
// miss in Governance. AcceptedAt is fixed by Governance, not this resolver.
type AdmissionResolutionRequest struct {
	TenantID   string
	Intent     cpup01.Intent
	Release    AdmissionReleaseSelection
	AcceptedAt time.Time
}

// TenantAdmissionFacts is explicit managed configuration, never user JSON.
// The supplying owner must establish the tenant association, revision digest,
// actual identities, storage authority and Runtime/workspace mount evidence.
// A typed value or a matching shape alone establishes none of those facts.
type TenantAdmissionFacts struct {
	TenantID           string
	Environment        cpup01.EnvironmentBindingSnapshot
	PublicationScope   cpup01.StorageScope
	InputScope         cpup01.StorageScope
	Runtime            cpup01.RuntimeRef
	Workspace          cpup01.WorkspaceContract
	InputFilePath      string
	OutputDirectoryPath string
}

// AdmissionResolution is an unpersisted candidate, not an acceptance receipt.
// Governance must bind real command IDs and commit its generation/key checks.
type AdmissionResolution struct {
	Snapshot cpup01.Snapshot
	SpecHash string
}

type AdmissionReleaseReader interface {
	ReadRelease(context.Context, string, string) (cpup01.ReleaseDocument, error)
}

type AdmissionInputReader interface {
	Get(context.Context, string, string) (InputVersion, error)
}

// AdmissionResolver owns fixed-fact composition, not authentication, current
// pointers, environment discovery, command identity allocation or persistence.
type AdmissionResolver struct {
	releases AdmissionReleaseReader
	inputs   AdmissionInputReader
}

func NewAdmissionResolver(releases AdmissionReleaseReader, inputs AdmissionInputReader) *AdmissionResolver {
	return &AdmissionResolver{releases: releases, inputs: inputs}
}

func (resolver *AdmissionResolver) Resolve(ctx context.Context, request AdmissionResolutionRequest, facts TenantAdmissionFacts) (AdmissionResolution, error) {
	return AdmissionResolution{}, errors.New("ADMISSION_RESOLUTION_NOT_IMPLEMENTED")
}
