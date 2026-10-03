// Package runtimeproof verifies managed step claims through current external
// APIs. Its observations must still commit through the execution repository.
package runtimeproof

import (
	"context"
	"errors"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/kfp"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/objectstore"
	"k8s.io/client-go/dynamic"
)

type PlanReader interface { Get(context.Context,string,string)(biz.PipelineDispatch,error) }
type Verifier struct {
	kube dynamic.Interface
	runs *kfp.Client
	objects *objectstore.Verifier
	plans PlanReader
}

func New(kube dynamic.Interface, runs *kfp.Client, objects *objectstore.Verifier, plans PlanReader) *Verifier {
	return &Verifier{kube:kube,runs:runs,objects:objects,plans:plans}
}

func (verifier *Verifier) VerifyPrepared(ctx context.Context, execution biz.Execution, association biz.ManagedStepAssociation, workspace biz.WorkspaceBinding) error {
	return errors.New("RUNTIME_PROOF_NOT_IMPLEMENTED")
}
func (verifier *Verifier) VerifyPublication(ctx context.Context, execution biz.Execution, association biz.ManagedStepAssociation, candidate biz.RuntimePublication) (biz.RuntimePublication,error) {
	return biz.RuntimePublication{},errors.New("RUNTIME_PROOF_NOT_IMPLEMENTED")
}

func (verifier *Verifier) VerifyWritersAbsent(ctx context.Context, execution biz.Execution, association biz.ManagedStepAssociation) (biz.ManagedCloseEvidence,error) {
	return biz.ManagedCloseEvidence{},errors.New("RUNTIME_PROOF_NOT_IMPLEMENTED")
}
