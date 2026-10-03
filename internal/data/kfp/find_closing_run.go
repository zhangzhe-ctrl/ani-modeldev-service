package kfp

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// FindClosingRun enumerates the frozen namespace/experiment through KFP 2.16
// ListRuns. A display name only selects candidates; GetManagedRun verifies the
// full immutable spec/version/root/service-account association for adoption.
// Empty, duplicate, incomplete or oversized results never prove non-creation.
func (client *Client) FindClosingRun(ctx context.Context, plan biz.PipelineDispatchPlan) (string, error) {
	if client == nil || client.http == nil || client.tokens == nil || ctx == nil || ctx.Err() != nil || plan.Environment.KFPConnectionRef != client.connectionRef {
		return "", ErrManagedRunUnverified
	}
	if _, err := plan.Digest(); err != nil {
		return "", ErrManagedRunUnverified
	}
	token, err := client.tokens.BearerToken(ctx, plan.TenantID, plan.Environment)
	if err != nil || len(token) > 16384 || !bearerTokenPattern.MatchString(token) {
		return "", ErrManagedRunUnverified
	}
	pageToken, found := "", ""
	seenPages, seenRuns := map[string]bool{}, map[string]bool{}
	for page := 0; page < 10; page++ {
		query := url.Values{"namespace": {plan.Environment.NamespaceName}, "experiment_id": {plan.Environment.ExperimentID}, "page_size": {"100"}}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.endpoint+"?"+query.Encode(), nil)
		if err != nil {
			return "", ErrManagedRunUnverified
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := client.http.Do(request)
		if err != nil {
			return "", ErrManagedRunUnverified
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxCreateRunResponseBytes+1))
		_ = response.Body.Close()
		mediaType, _, typeErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if response.StatusCode != http.StatusOK || readErr != nil || typeErr != nil || mediaType != "application/json" || len(body) > maxCreateRunResponseBytes {
			return "", ErrManagedRunUnverified
		}
		var result struct {
			Runs []struct {
				RunID       string          `json:"run_id"`
				DisplayName string          `json:"display_name"`
				Error       json.RawMessage `json:"error"`
			} `json:"runs"`
			NextPageToken string `json:"next_page_token"`
		}
		if json.Unmarshal(body, &result) != nil || len(result.Runs) > 100 {
			return "", ErrManagedRunUnverified
		}
		for _, run := range result.Runs {
			id, err := uuid.Parse(run.RunID)
			if err != nil || id == uuid.Nil || id.String() != run.RunID || run.DisplayName == "" || seenRuns[run.RunID] || (len(run.Error) > 0 && string(run.Error) != "null") {
				return "", ErrManagedRunUnverified
			}
			seenRuns[run.RunID] = true
			if run.DisplayName != plan.DisplayName {
				continue
			}
			if _, err := client.GetManagedRun(ctx, plan, run.RunID); err != nil {
				return "", err
			}
			if found != "" {
				return "", &biz.AmbiguousClosingRunsError{RunIDs: []string{found, run.RunID}}
			}
			found = run.RunID
		}
		if result.NextPageToken == "" {
			if found == "" {
				return "", biz.ErrPipelineSubmissionUncertain
			}
			return found, nil
		}
		if len(result.NextPageToken) > 4096 || seenPages[result.NextPageToken] {
			return "", ErrManagedRunUnverified
		}
		seenPages[result.NextPageToken], pageToken = true, result.NextPageToken
	}
	return "", ErrManagedRunUnverified
}
