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

// ManagedTask is an observation from KFP's frozen Run, not a callback claim.
// DAG tasks without Pods are retained but cannot prove a writer's identity.
type ManagedTask struct {
	RunID, ID, Name, PodName, State string
}

// VerifyManagedRun reads KFP's own Run/task association, independently of
// caller claims and Kubernetes labels. It never creates or retries a Run.
func (client *Client) VerifyManagedRun(ctx context.Context, plan biz.PipelineDispatchPlan, association biz.ManagedStepAssociation, taskName string) error {
	if len(validation.IsDNS1123Subdomain(association.PodName)) != 0 || taskName == "" {
		return ErrManagedRunUnverified
	}
	tasks, err := client.GetManagedTasks(ctx, plan, association.RunID)
	if err != nil { return err }
	matches := 0
	for _, task := range tasks {
		if task.PodName != association.PodName { continue }
		if task.RunID != association.RunID || task.Name != taskName { return ErrManagedRunUnverified }
		matches++
	}
	if matches != 1 { return ErrManagedRunUnverified }
	return nil
}

// GetManagedTasks always reads the actual authenticated GetRun endpoint and
// verifies the complete frozen execution/spec/version/root/SA association.
func (client *Client) GetManagedTasks(ctx context.Context, plan biz.PipelineDispatchPlan, runID string) ([]ManagedTask,error) {
	id, err := uuid.Parse(runID)
	if client == nil || client.http == nil || client.tokens == nil || ctx == nil || ctx.Err() != nil ||
		err != nil || id == uuid.Nil || id.String() != runID || plan.Environment.KFPConnectionRef != client.connectionRef {
		return nil,ErrManagedRunUnverified
	}
	token, err := client.tokens.BearerToken(ctx, plan.TenantID, plan.Environment)
	if err != nil || len(token) > 16384 || !bearerTokenPattern.MatchString(token) {
		return nil,ErrManagedRunUnverified
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.endpoint+"/"+id.String(), nil)
	if err != nil {
		return nil,ErrManagedRunUnverified
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.http.Do(request)
	if err != nil {
		return nil,ErrManagedRunUnverified
	}
	defer response.Body.Close()
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK || err != nil || mediaType != "application/json" {
		return nil,ErrManagedRunUnverified
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxCreateRunResponseBytes+1))
	if err != nil || len(body) > maxCreateRunResponseBytes {
		return nil,ErrManagedRunUnverified
	}
	expected, err := json.Marshal(createRunBody{
		ExperimentID: strings.ToLower(plan.Environment.ExperimentID), DisplayName: plan.DisplayName,
		PipelineVersionReference: pipelineVersionReference{PipelineID: strings.ToLower(plan.PipelineID), PipelineVersionID: strings.ToLower(plan.PipelineVersionID)},
		RuntimeConfig:            runtimeConfig{Parameters: map[string]string{"execution_id": plan.ExecutionID, "spec_hash": plan.SpecHash}, PipelineRoot: plan.Owner.PipelineRoot},
		ServiceAccount:           plan.Environment.Identities.KFPStepServiceAccount,
	})
	if err != nil || confirmedRunID(body, expected) != runID {
		return nil,ErrManagedRunUnverified
	}
	// KFP 2.16.0 run.proto supplies task_details.run_id/display_name/pod_name.
	// A Pod label or a callback's claimed Run is never the association source.
	var run map[string]any
	if json.Unmarshal(body, &run) != nil {
		return nil,ErrManagedRunUnverified
	}
	details, ok := run["run_details"].(map[string]any)
	if !ok {
		return nil,ErrManagedRunUnverified
	}
	tasks, ok := details["task_details"].([]any)
	if !ok {
		return nil,ErrManagedRunUnverified
	}
	result := make([]ManagedTask,0,len(tasks))
	for _, value := range tasks {
		task, ok := value.(map[string]any)
		if !ok {
			return nil,ErrManagedRunUnverified
		}
		observed := ManagedTask{}
		for key,target := range map[string]*string{"run_id":&observed.RunID,"task_id":&observed.ID,"display_name":&observed.Name,"pod_name":&observed.PodName,"state":&observed.State} {
			if value,present := task[key]; present { text,ok := value.(string); if !ok { return nil,ErrManagedRunUnverified }; *target = text }
		}
		if observed.PodName != "" && (observed.RunID != runID || len(validation.IsDNS1123Subdomain(observed.PodName)) != 0 || observed.Name == "") { return nil,ErrManagedRunUnverified }
		result = append(result,observed)
	}
	return result,nil
}
