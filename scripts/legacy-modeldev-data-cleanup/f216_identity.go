package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func verifyF216Cluster(ctx context.Context) error {
	hostname, err := os.Hostname()
	if err != nil || hostname != "node1" || os.Getenv("SUDO_USER") != "kubercloud" && os.Getenv("USER") != "kubercloud" {
		return fmt.Errorf("F216_SOLE_SSH_HOST_IDENTITY_DIFFERS")
	}
	call := func(args ...string) ([]byte, error) {
		bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		raw, err := exec.CommandContext(bounded, "kubectl", append([]string{"--request-timeout=10s"}, args...)...).Output()
		if err != nil || len(raw) > 2<<20 {
			return nil, fmt.Errorf("F216_FULL_CLUSTER_IDENTITY_UNVERIFIED")
		}
		return raw, nil
	}
	contextRaw, err := call("config", "current-context")
	if err != nil || strings.TrimSpace(string(contextRaw)) != "kubernetes-admin@ani-kcn-full" {
		return fmt.Errorf("F216_CLUSTER_CONTEXT_DIFFERS")
	}
	raw, err := call("config", "view", "--minify", "-o", "json")
	if err != nil {
		return err
	}
	var config struct {
		Clusters []struct{ Cluster struct{ Server string } }
	}
	if json.Unmarshal(raw, &config) != nil || len(config.Clusters) != 1 || config.Clusters[0].Cluster.Server != "https://lb.kubesphere.local:6443" {
		return fmt.Errorf("F216_API_SERVER_DIFFERS")
	}
	raw, err = call("get", "ns", "kube-system", "-o", "json")
	if err != nil {
		return err
	}
	var ns struct{ Metadata struct{ UID string } }
	if json.Unmarshal(raw, &ns) != nil || ns.Metadata.UID != "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24" {
		return fmt.Errorf("F216_KUBE_SYSTEM_UID_DIFFERS")
	}
	raw, err = call("-n", "ani-system", "get", "secret", "ani-resource-private", "-o", "json")
	if err != nil {
		return err
	}
	var secret struct{ Data map[string]string }
	if json.Unmarshal(raw, &secret) != nil {
		return fmt.Errorf("F216_CLUSTER_ID_UNVERIFIED")
	}
	source, err := base64.StdEncoding.DecodeString(secret.Data["config.yaml"])
	if err != nil {
		return fmt.Errorf("F216_CLUSTER_ID_UNVERIFIED")
	}
	matches := regexp.MustCompile(`(?m)^\s*cluster_id:\s*["']?([a-f0-9-]{36})`).FindAllSubmatch(source, -1)
	if len(matches) != 1 || string(matches[0][1]) != "f39bf6e2-f3ed-4191-ada3-caeebf86c220" {
		return fmt.Errorf("F216_CLUSTER_ID_DIFFERS")
	}
	return nil
}
