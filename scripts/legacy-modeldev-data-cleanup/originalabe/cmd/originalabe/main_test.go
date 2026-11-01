package main

import (
	"context"
	"net/http"
	"testing"
)

type mustNotCall struct{ t *testing.T }

func (m mustNotCall) RoundTrip(*http.Request) (*http.Response, error) {
	m.t.Fatal("mutation/log request reached network")
	return nil, nil
}
func TestGetOnlyTransportRejectsAllMutationsAndLogs(t *testing.T) {
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE", "GET"} {
		path := "https://kubernetes.default.svc/api/v1/namespaces/x/pods/p"
		if method == "GET" {
			path += "/log"
		}
		r, _ := http.NewRequestWithContext(context.Background(), method, path, nil)
		if _, e := (getOnly{mustNotCall{t}}).RoundTrip(r); e == nil {
			t.Fatal("forbidden request accepted", method)
		}
	}
}
