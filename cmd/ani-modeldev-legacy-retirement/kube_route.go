package main

import "errors"

const originalAPIServiceUID = "bfbd6463-67e2-471c-8d49-a84b853bd4e0"
const originalAPIServiceIP = "10.96.0.1"

func originalAPITarget(address string) (string, error) {
	if address != "kubernetes.default.svc:443" {
		return "", errors.New("MAINTENANCE_ORIGINAL_API_ROUTE_INVALID")
	}
	return originalAPIServiceIP + ":443", nil
}

func validOriginalAPIService(object map[string]any) bool {
	meta, _ := object["metadata"].(map[string]any)
	spec, _ := object["spec"].(map[string]any)
	ports, _ := spec["ports"].([]any)
	if object["apiVersion"] != "v1" || object["kind"] != "Service" || meta["namespace"] != "default" || meta["name"] != "kubernetes" || meta["uid"] != originalAPIServiceUID || meta["deletionTimestamp"] != nil || spec["type"] != "ClusterIP" || spec["clusterIP"] != originalAPIServiceIP || len(ports) != 1 {
		return false
	}
	port, _ := ports[0].(map[string]any)
	return port["port"] == float64(443) && port["protocol"] == "TCP"
}
