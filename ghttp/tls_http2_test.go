package ghttp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/logging"
)

func TestRunContextTLSNegotiatesHTTP2(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	server := NewServer(
		WithAddress("127.0.0.1:0"),
		WithTLS(certFile, keyFile),
		WithLogger(logging.NewNopLogger()),
	)
	server.Engine().GET("/protocol", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, ctx.Request.Proto)
	})

	address := make(chan string, 1)
	server.OnStarted(func(context.Context) error {
		server.listenerMu.RLock()
		listener := server.listener
		server.listenerMu.RUnlock()
		if listener == nil {
			return nil
		}
		address <- listener.Addr().String()
		return nil
	})

	runContext, cancel := context.WithCancel(context.Background())
	runError := make(chan error, 1)
	go func() { runError <- server.RunContext(runContext) }()

	var addr string
	select {
	case addr = <-address:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("TLS server did not reach started state")
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // test-only certificate
		ForceAttemptHTTP2: true,
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Get("https://" + addr + "/protocol")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.ProtoMajor != 2 {
		cancel()
		t.Fatalf("negotiated protocol %q, want HTTP/2", response.Proto)
	}

	cancel()
	transport.CloseIdleConnections()
	if err := <-runError; err != nil {
		t.Fatalf("RunContext returned %v", err)
	}
}

func writeTestCertificate(t *testing.T) (string, string) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "gonex-test"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	certFile := filepath.Join(directory, "cert.pem")
	keyFile := filepath.Join(directory, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
