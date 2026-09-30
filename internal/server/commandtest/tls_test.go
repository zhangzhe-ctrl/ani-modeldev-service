package commandtest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestGovernanceCommandRejectsUntrustedTLSBeforePersistence(t *testing.T) {
	openPool := postgres.Prepare(t)
	certificates, otherAuthority := commandtls.New(t), commandtls.New(t)
	address, _ := startCommandListener(t, execution.New(openPool()), certificates)
	reader := execution.New(openPool())
	cases := []struct {
		name   string
		change func(*testing.T, *tls.Config)
	}{
		{"no client certificate", func(_ *testing.T, c *tls.Config) { c.Certificates = nil }},
		{"other client CA", func(_ *testing.T, c *tls.Config) { c.Certificates = []tls.Certificate{otherAuthority.Governance} }},
		{"wrong client EKU", func(t *testing.T, c *tls.Config) {
			c.Certificates = []tls.Certificate{certificates.ClientCertificate(t, func(leaf *x509.Certificate) { leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} })}
		}},
		{"same CA ordinary training SAN", func(t *testing.T, c *tls.Config) {
			c.Certificates = []tls.Certificate{certificates.ClientCertificate(t, func(leaf *x509.Certificate) { leaf.DNSNames = []string{"ordinary-training"} })}
		}},
		{"CN only impersonation", func(t *testing.T, c *tls.Config) {
			c.Certificates = []tls.Certificate{certificates.ClientCertificate(t, func(leaf *x509.Certificate) { leaf.DNSNames = nil; leaf.Subject.CommonName = commandtls.GovernanceDNSName })}
		}},
		{"multiple DNS SANs", func(t *testing.T, c *tls.Config) {
			c.Certificates = []tls.Certificate{certificates.ClientCertificate(t, func(leaf *x509.Certificate) { leaf.DNSNames = []string{commandtls.GovernanceDNSName, "ordinary-training"} })}
		}},
		{"DNS and URI identity", func(t *testing.T, c *tls.Config) {
			c.Certificates = []tls.Certificate{certificates.ClientCertificate(t, func(leaf *x509.Certificate) { leaf.URIs = []*url.URL{{Scheme: "spiffe", Host: "test", Path: "/ordinary-training"}} })}
		}},
		{"wildcard DNS SAN", func(t *testing.T, c *tls.Config) {
			c.Certificates = []tls.Certificate{certificates.ClientCertificate(t, func(leaf *x509.Certificate) { leaf.DNSNames = []string{"*.ani-governance"} })}
		}},
		{"expired certificate", func(t *testing.T, c *tls.Config) {
			c.Certificates = []tls.Certificate{certificates.ClientCertificate(t, func(leaf *x509.Certificate) { leaf.NotAfter = time.Now().Add(-time.Second) })}
		}},
		{"not yet valid certificate", func(t *testing.T, c *tls.Config) {
			c.Certificates = []tls.Certificate{certificates.ClientCertificate(t, func(leaf *x509.Certificate) { leaf.NotBefore = time.Now().Add(time.Minute) })}
		}},
		{"wrong server identity", func(_ *testing.T, c *tls.Config) { c.ServerName = "other-modeldev.test" }},
		{"untrusted server CA", func(_ *testing.T, c *tls.Config) { c.RootCAs = otherAuthority.Roots }},
		{"TLS 1.2", func(_ *testing.T, c *tls.Config) { c.MinVersion, c.MaxVersion = tls.VersionTLS12, tls.VersionTLS12 }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			config := commandClientTLS(certificates)
			testCase.change(t, config)
			client := modeldevv1.NewModelDevCommandServiceClient(commandConnection(t, address, config))
			request := validCloseRequest()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			response, err := client.ApplyCloseIntent(commandContext(ctx, request), request)
			if response != nil || (status.Code(err) != codes.Unauthenticated && status.Code(err) != codes.Unavailable) {
				t.Errorf("untrusted transport returned code %s, ACK=%t", status.Code(err), response != nil)
			}
			assertNoCommandFacts(t, reader, request)
		})
	}
	t.Run("plaintext original port", func(t *testing.T) {
		connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatal("plaintext negative fixture could not dial")
		}
		defer connection.Close()
		request := validCloseRequest()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		response, err := modeldevv1.NewModelDevCommandServiceClient(connection).ApplyCloseIntent(commandContext(ctx, request), request)
		if response != nil || status.Code(err) != codes.Unavailable {
			t.Errorf("plaintext transport returned code %s, ACK=%t", status.Code(err), response != nil)
		}
		assertNoCommandFacts(t, reader, request)
	})
	// The same listener must still accept its real authorized TLS control; a
	// stopped or broken service cannot make the negative matrix appear secure.
	request := validCloseRequest()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := modeldevv1.NewModelDevCommandServiceClient(commandConnection(t, address, commandClientTLS(certificates)))
	response, err := client.ApplyCloseIntent(commandContext(ctx, request), request)
	if err != nil {
		t.Fatalf("authorized TLS control failed after negative matrix: %v", err)
	}
	assertCloseResponse(t, response, request, false)
}

func TestGovernanceCommandRechecksCertificateExpiryOnExistingConnection(t *testing.T) {
	openPool := postgres.Prepare(t)
	certificates := commandtls.New(t)
	address, _ := startCommandListener(t, execution.New(openPool()), certificates)
	shortLived := certificates.ClientCertificate(t, func(leaf *x509.Certificate) {
		leaf.NotAfter = time.Now().Truncate(time.Second).Add(3 * time.Second)
	})
	config := commandClientTLS(certificates)
	config.Certificates = nil
	var handshakes atomic.Int32
	config.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		handshakes.Add(1)
		return &shortLived, nil
	}
	client := modeldevv1.NewModelDevCommandServiceClient(commandConnection(t, address, config))
	first := validCloseRequest()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	response, err := client.ApplyCloseIntent(commandContext(ctx, first), first)
	if err != nil {
		t.Fatalf("short-lived certificate fixture failed before expiry; behavior NOT_RUN: %v", err)
	}
	assertCloseResponse(t, response, first, false)
	if handshakes.Load() != 1 || !time.Now().Before(shortLived.Leaf.NotAfter) {
		t.Fatal("expiry fixture lacks one still-current established connection; behavior NOT_RUN")
	}
	timer := time.NewTimer(time.Until(shortLived.Leaf.NotAfter) + 50*time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("bounded expiry fixture did not complete")
	}
	second := validCloseRequest()
	response, err = client.ApplyCloseIntent(commandContext(ctx, second), second)
	if response != nil || status.Code(err) != codes.Unauthenticated {
		t.Errorf("expired existing connection returned code %s, ACK=%t", status.Code(err), response != nil)
	}
	if handshakes.Load() != 1 {
		t.Error("expiry denial must be verified on the existing connection, without another handshake")
	}
	reader := execution.New(openPool())
	assertNoCommandFacts(t, reader, second)
	if stored, err := reader.GetCloseIntent(ctx, first.ResourceTenantId, first.Identity.ExecutionId); err != nil || stored.Generation != 1 {
		t.Fatalf("expiry rejection changed the earlier committed fact: %v", err)
	}
}

func assertNoCommandFacts(t *testing.T, reader *execution.Repository, request *modeldevv1.ApplyCloseIntentRequest) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := reader.GetCloseIntent(ctx, request.ResourceTenantId, request.Identity.ExecutionId); !errors.Is(err, biz.ErrExecutionNotFound) {
		t.Errorf("rejected command left a close fact: %v", err)
	}
	if _, err := reader.Get(ctx, request.ResourceTenantId, request.Identity.ExecutionId); !errors.Is(err, biz.ErrExecutionNotFound) {
		t.Errorf("rejected command created an Admission: %v", err)
	}
}
