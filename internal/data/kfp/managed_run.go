package kfp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"k8s.io/apimachinery/pkg/util/validation"
)

var ErrManagedRunUnverified = errors.New("KFP_MANAGED_RUN_UNVERIFIED")

// VerifyManagedRun reads KFP's own Run/task association, independently of
// caller claims and Kubernetes labels. It never creates or retries a Run.
func (client *Client) VerifyManagedRun(ctx context.Context, plan biz.PipelineDispatchPlan, association biz.ManagedStepAssociation, taskName string) error {
	id, err := uuid.Parse(association.RunID)
	if client == nil || client.http == nil || client.tokens == nil || ctx == nil || ctx.Err() != nil ||
		err != nil || id == uuid.Nil || id.String() != association.RunID || plan.Environment.KFPConnectionRef != client.connectionRef ||
		len(validation.IsDNS1123Subdomain(association.PodName)) != 0 || taskName == "" {
		return ErrManagedRunUnverified
	}
	token, err := client.tokens.BearerToken(ctx, plan.TenantID, plan.Environment)
	if err != nil || len(token) > 16384 || !bearerTokenPattern.MatchString(token) {
		return ErrManagedRunUnverified
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.endpoint+"/"+id.String(), nil)
	if err != nil {
		return ErrManagedRunUnverified
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.http.Do(request)
	if err != nil {
		return ErrManagedRunUnverified
	}
	defer response.Body.Close()
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK || err != nil || mediaType != "application/json" {
		return ErrManagedRunUnverified
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxCreateRunResponseBytes+1))
	if err != nil || len(body) > maxCreateRunResponseBytes {
		return ErrManagedRunUnverified
	}
	expected, err := json.Marshal(createRunBody{
		ExperimentID: strings.ToLower(plan.Environment.ExperimentID), DisplayName: plan.DisplayName,
		PipelineVersionReference: pipelineVersionReference{PipelineID: strings.ToLower(plan.PipelineID), PipelineVersionID: strings.ToLower(plan.PipelineVersionID)},
		RuntimeConfig:            runtimeConfig{Parameters: map[string]string{"execution_id": plan.ExecutionID, "spec_hash": plan.SpecHash}, PipelineRoot: plan.Owner.PipelineRoot},
		ServiceAccount:           plan.Environment.Identities.KFPStepServiceAccount,
	})
	if err != nil || confirmedRunID(body, expected) != association.RunID {
		return ErrManagedRunUnverified
	}
	// KFP 2.16.0 run.proto supplies task_details.run_id/display_name/pod_name.
	// A Pod label or a callback's claimed Run is never the association source.
	var run map[string]any
	if json.Unmarshal(body, &run) != nil {
		return ErrManagedRunUnverified
	}
	details, ok := run["run_details"].(map[string]any)
	if !ok {
		return ErrManagedRunUnverified
	}
	tasks, ok := details["task_details"].([]any)
	if !ok {
		return ErrManagedRunUnverified
	}
	matches := 0
	for _, value := range tasks {
		task, ok := value.(map[string]any)
		if !ok {
			return ErrManagedRunUnverified
		}
		if task["pod_name"] != association.PodName {
			continue
		}
		if task["run_id"] != association.RunID || task["display_name"] != taskName {
			return ErrManagedRunUnverified
		}
		matches++
	}
	if matches != 1 {
		return ErrManagedRunUnverified
	}
	return nil
}
