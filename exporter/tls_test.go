// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func TestConfigValidateTLSFilesOK(t *testing.T) {
	t.Parallel()

	caPath, certPath, keyPath := writeSelfSignedTLSFiles(t)
	cfg := validTLSConfig()
	cfg.Certificate = caPath
	cfg.ClientCertificate = certPath
	cfg.ClientKey = keyPath

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("buildTLSConfig() error = %v, want nil", err)
	}
	if tlsConfig == nil || tlsConfig.RootCAs == nil || len(tlsConfig.Certificates) != 1 {
		t.Fatalf("buildTLSConfig() = %#v, want root CAs and one client certificate", tlsConfig)
	}
}

func TestConfigValidateTLSErrors(t *testing.T) {
	t.Parallel()

	caPath, certPath, keyPath := writeSelfSignedTLSFiles(t)
	badCA := filepath.Join(t.TempDir(), "bad-ca.pem")
	if err := os.WriteFile(badCA, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	tests := []struct {
		name  string
		cfg   Config
		field string
	}{
		{name: "insecure with certificate", cfg: withConfig(validTLSConfig(), func(c *Config) {
			c.Insecure = true
			c.Certificate = caPath
		}), field: "Insecure"},
		{name: "client cert without key", cfg: withConfig(validTLSConfig(), func(c *Config) {
			c.ClientCertificate = certPath
		}), field: "ClientCertificate"},
		{name: "client key without cert", cfg: withConfig(validTLSConfig(), func(c *Config) {
			c.ClientKey = keyPath
		}), field: "ClientCertificate"},
		{name: "bad ca", cfg: withConfig(validTLSConfig(), func(c *Config) {
			c.Certificate = badCA
		}), field: "Certificate"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want TLS validation error")
			}
			if !joinedErrorHasField(err, tt.field) {
				t.Fatalf("Validate() error = %v, want field %s", err, tt.field)
			}
		})
	}
}

func TestConfigValidateTLSInvalidCombosProperty(t *testing.T) {
	t.Parallel()

	caPath, certPath, keyPath := writeSelfSignedTLSFiles(t)
	missingCA := filepath.Join(t.TempDir(), "missing-ca.pem")
	rapid.Check(t, func(t *rapid.T) {
		cfg := validTLSConfig()
		switch rapid.IntRange(0, 3).Draw(t, "invalid_tls_combo") {
		case 0:
			cfg.Insecure = true
			cfg.Certificate = caPath
		case 1:
			cfg.ClientCertificate = certPath
		case 2:
			cfg.ClientKey = keyPath
		case 3:
			cfg.Certificate = missingCA
		}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate() error = nil for invalid TLS config %#v", cfg)
		}
	})
}

func TestBuildTLSConfigNoCertificates(t *testing.T) {
	t.Parallel()

	tlsConfig, err := buildTLSConfig(validTLSConfig())
	if err != nil {
		t.Fatalf("buildTLSConfig() error = %v, want nil", err)
	}
	if tlsConfig != nil {
		t.Fatalf("buildTLSConfig() = %#v, want nil without certificate options", tlsConfig)
	}
}

func validTLSConfig() Config {
	return Config{
		Protocol:          ProtocolGRPC,
		Endpoint:          "localhost:4317",
		Timeout:           time.Second,
		BatchSize:         16,
		BatchTimeout:      time.Second,
		MaxQueueSize:      32,
		Sampler:           "always_on",
		SamplerArg:        1,
		SamplerArgSet:     true,
		ResourceOverrides: nil,
	}
}

func writeSelfSignedTLSFiles(t testing.TB) (caPath, certPath, keyPath string) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "xk6-otel-gen-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if !strings.Contains(string(certPEM), "CERTIFICATE") || !strings.Contains(string(keyPEM), "PRIVATE KEY") {
		t.Fatal("generated PEM files are malformed")
	}

	dir := t.TempDir()
	caPath = filepath.Join(dir, "ca.pem")
	certPath = filepath.Join(dir, "client.pem")
	keyPath = filepath.Join(dir, "client-key.pem")
	for path, content := range map[string][]byte{
		caPath:   certPEM,
		certPath: certPEM,
		keyPath:  keyPEM,
	} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	return caPath, certPath, keyPath
}
