package main

import (
	"context"
	"errors"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"k8s.io/client-go/dynamic"
)

type invocation struct {
	step string
	config ownerConfig
	claim *modeldevv1.StepContext
	candidateJSON *string
}

// bootstrapInvocation is the CLI input boundary used before any business RPC.
// The supplied client is the official in-cluster Kubernetes client in execute.
func bootstrapInvocation(context.Context,[]string,dynamic.Interface) (invocation,error) {
	return invocation{},errors.New("KFP_COMPONENT_BOOTSTRAP_NOT_IMPLEMENTED")
}
