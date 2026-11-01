package main

import (
	"encoding/json"
	"testing"
)

func TestLegacyRetirementAPITransportKeepsExactOriginalService(t *testing.T) {
	for _, address := range []string{"kubernetes.default.svc:443", "kubernetes.default.svc:6443", "other.default.svc:443", "10.96.0.1:443", "kubernetes.default.svc.cluster.local:443", ""} {
		target, err := originalAPITarget(address)
		if address == "kubernetes.default.svc:443" {
			if err != nil || target != "10.96.0.1:443" {
				t.Fatal("original endpoint not routed to its verified Service")
			}
		} else if err == nil || target != "" {
			t.Fatal("alternate endpoint admitted", address)
		}
	}
	raw := []byte(`{"apiVersion":"v1","kind":"Service","metadata":{"namespace":"default","name":"kubernetes","uid":"bfbd6463-67e2-471c-8d49-a84b853bd4e0"},"spec":{"type":"ClusterIP","clusterIP":"10.96.0.1","ports":[{"port":443,"protocol":"TCP"}]}}`)
	for _, attack := range []string{"none", "uid", "namespace", "deleted", "ip", "type", "port", "protocol", "extra-port", "kind"} {
		var object map[string]any
		if err := json.Unmarshal(raw, &object); err != nil {
			t.Fatal(err)
		}
		meta := object["metadata"].(map[string]any)
		spec := object["spec"].(map[string]any)
		port := spec["ports"].([]any)[0].(map[string]any)
		switch attack {
		case "uid":
			meta["uid"] = "replacement"
		case "namespace":
			meta["namespace"] = "other"
		case "deleted":
			meta["deletionTimestamp"] = "2026-11-01T00:00:00Z"
		case "ip":
			spec["clusterIP"] = "10.96.0.2"
		case "type":
			spec["type"] = "ExternalName"
		case "port":
			port["port"] = float64(6443)
		case "protocol":
			port["protocol"] = "UDP"
		case "extra-port":
			spec["ports"] = append(spec["ports"].([]any), port)
		case "kind":
			object["kind"] = "Secret"
		}
		if validOriginalAPIService(object) != (attack == "none") {
			t.Fatal("original Service identity qualification", attack)
		}
	}
}
