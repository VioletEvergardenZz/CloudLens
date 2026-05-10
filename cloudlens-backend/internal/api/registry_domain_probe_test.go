package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeRegistryDomainProbeRunner struct {
	result     registryDomainProbeResult
	err        error
	lastTarget string
}

func (f *fakeRegistryDomainProbeRunner) Probe(_ context.Context, rawTarget string) (registryDomainProbeResult, error) {
	f.lastTarget = rawTarget
	return f.result, f.err
}

func allowAnyRegistryProbeTarget(context.Context, string) error {
	return nil
}

func decodeRegistryDomainProbeResponse(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	return payload
}

func TestNormalizeRegistryDomainProbeTarget_StripsPathAndQuery(t *testing.T) {
	got, err := normalizeRegistryDomainProbeTarget("https://demo.example.com/portal/index.html?debug=1")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if got != "demo.example.com" {
		t.Fatalf("expected demo.example.com, got %s", got)
	}
}

func TestRegistryDomainProbeHandler_MethodNotAllowed(t *testing.T) {
	h := &handler{}

	req := httptest.NewRequest(http.MethodDelete, "/api/registry/domain-probe", nil)
	rec := httptest.NewRecorder()
	h.registryDomainProbe(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestRegistryDomainProbeHandler_GETSuccess(t *testing.T) {
	runner := &fakeRegistryDomainProbeRunner{
		result: registryDomainProbeResult{
			Domain:              "demo.example.com",
			NormalizedTarget:    "demo.example.com",
			Reachable:           true,
			RecommendedBaseURL:  "https://demo.example.com",
			SuggestedAccessType: registryAccessTypeFrontendOnly,
		},
	}
	h := &handler{domainProber: runner}

	req := httptest.NewRequest(http.MethodGet, "/api/registry/domain-probe?domain=demo.example.com", nil)
	rec := httptest.NewRecorder()
	h.registryDomainProbe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}
	if runner.lastTarget != "demo.example.com" {
		t.Fatalf("expected runner to receive target, got %q", runner.lastTarget)
	}
	payload := decodeRegistryDomainProbeResponse(t, rec.Body.Bytes())
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %#v", payload["ok"])
	}
}

func TestRegistryDomainProbeHandler_POSTInvalidPayload(t *testing.T) {
	h := &handler{}

	req := httptest.NewRequest(http.MethodPost, "/api/registry/domain-probe", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	h.registryDomainProbe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestRegistryDomainProber_RejectsIPTargetByDefault(t *testing.T) {
	prober := newRegistryDomainProber(registryDomainProbeOptions{})
	_, err := prober.Probe(context.Background(), "127.0.0.1:8080")
	if !errors.Is(err, errRegistryDomainProbeInvalidTarget) {
		t.Fatalf("expected invalid target error, got %v", err)
	}
}

func TestRegistryDomainProber_ClassifiesFrontendBackendIntegrated(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Marketing Hub</title></head><body><div id="root"></div></body></html>`))
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()

	prober := newRegistryDomainProber(registryDomainProbeOptions{
		ValidateTarget: allowAnyRegistryProbeTarget,
	})
	result, err := prober.Probe(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}

	if !result.Reachable {
		t.Fatalf("expected probe to be reachable")
	}
	if result.SuggestedAccessType != registryAccessTypeFrontendBackendIntegrated {
		t.Fatalf("expected %s, got %s", registryAccessTypeFrontendBackendIntegrated, result.SuggestedAccessType)
	}
	if result.TLS == nil {
		t.Fatalf("expected tls info")
	}
	if result.TLS.Status != "ok" && result.TLS.Status != "expiring" {
		t.Fatalf("expected tls status ok/expiring, got %s", result.TLS.Status)
	}
	if !hasLikelyRegistryHealthCandidate(result.HealthCandidates) {
		t.Fatalf("expected at least one likely health candidate")
	}
	if !containsRegistryPendingCode(result.PendingItems, "confirm_health_endpoint") {
		t.Fatalf("expected confirm_health_endpoint pending item")
	}
	if !containsRegistryPendingCode(result.PendingItems, "confirm_frontend_source") {
		t.Fatalf("expected confirm_frontend_source pending item")
	}
}

func TestRegistryDomainProber_ClassifiesAPIOnly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"service":"demo-api"}`))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	prober := newRegistryDomainProber(registryDomainProbeOptions{
		ValidateTarget: allowAnyRegistryProbeTarget,
	})
	result, err := prober.Probe(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}

	if result.SuggestedAccessType != registryAccessTypeAPIOnly {
		t.Fatalf("expected %s, got %s", registryAccessTypeAPIOnly, result.SuggestedAccessType)
	}
	if !containsRegistryPendingCode(result.PendingItems, "confirm_backend_runtime") {
		t.Fatalf("expected confirm_backend_runtime pending item")
	}
}

func TestRegistryDomainProber_ProbeEndpointFollowsHTTPToHTTPS(t *testing.T) {
	now := time.Now().UTC()
	tlsMux := http.NewServeMux()
	tlsMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Portal</title></head><body>ok</body></html>`))
	})
	tlsServer := httptest.NewUnstartedServer(tlsMux)
	tlsServer.TLS = &tls.Config{
		Certificates: []tls.Certificate{newRegistryProbeTestCert(t, now, now.Add(20*24*time.Hour))},
	}
	tlsServer.StartTLS()
	defer tlsServer.Close()

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, tlsServer.URL, http.StatusMovedPermanently)
	}))
	defer httpServer.Close()

	prober := newRegistryDomainProber(registryDomainProbeOptions{
		ValidateTarget: allowAnyRegistryProbeTarget,
		Now:            func() time.Time { return now },
	})
	result := prober.probeEndpoint(context.Background(), httpServer.URL)

	if !result.Redirected {
		t.Fatalf("expected http probe to record redirect")
	}
	if !strings.HasPrefix(strings.ToLower(result.FinalURL), "https://") {
		t.Fatalf("expected http final url to be https, got %s", result.FinalURL)
	}
	if result.TLS == nil || result.TLS.Status != "expiring" {
		t.Fatalf("expected expiring tls status, got %#v", result.TLS)
	}
}

func TestRegistryDomainProber_FlagsExpiringCertInPendingItems(t *testing.T) {
	now := time.Now().UTC()
	tlsMux := http.NewServeMux()
	tlsMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Portal</title></head><body>ok</body></html>`))
	})
	tlsServer := httptest.NewUnstartedServer(tlsMux)
	tlsServer.TLS = &tls.Config{
		Certificates: []tls.Certificate{newRegistryProbeTestCert(t, now, now.Add(20*24*time.Hour))},
	}
	tlsServer.StartTLS()
	defer tlsServer.Close()

	prober := newRegistryDomainProber(registryDomainProbeOptions{
		ValidateTarget: allowAnyRegistryProbeTarget,
		Now:            func() time.Time { return now },
	})
	result, err := prober.Probe(context.Background(), tlsServer.URL)
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}

	if result.TLS == nil || result.TLS.Status != "expiring" {
		t.Fatalf("expected expiring tls status, got %#v", result.TLS)
	}
	if !containsRegistryPendingCode(result.PendingItems, "confirm_tls_alert") {
		t.Fatalf("expected confirm_tls_alert pending item")
	}
}

func containsRegistryPendingCode(items []registryDomainPendingItem, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func newRegistryProbeTestCert(t *testing.T, notBefore, notAfter time.Time) tls.Certificate {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
		},
		DNSNames: []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate failed: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("load x509 key pair failed: %v", err)
	}
	return certificate
}
