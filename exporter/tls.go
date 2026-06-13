// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

func validateTLSConfig(cfg Config) error {
	var errs []error
	if cfg.Insecure && (cfg.Certificate != "" || cfg.ClientCertificate != "" || cfg.ClientKey != "") {
		errs = append(errs, &ConfigError{Field: "Insecure", Value: true, Message: "must be false when TLS certificates are configured"})
	}
	if (cfg.ClientCertificate == "") != (cfg.ClientKey == "") {
		errs = append(errs, &ConfigError{Field: "ClientCertificate", Value: cfg.ClientCertificate, Message: "client certificate and client key must be configured together"})
	}
	if cfg.Certificate != "" {
		if _, err := loadRootCAs(cfg.Certificate); err != nil {
			errs = append(errs, &ConfigError{Field: "Certificate", Value: cfg.Certificate, Message: err.Error()})
		}
	}
	if cfg.ClientCertificate != "" && cfg.ClientKey != "" {
		if _, err := tls.LoadX509KeyPair(cfg.ClientCertificate, cfg.ClientKey); err != nil {
			errs = append(errs, &ConfigError{Field: "ClientCertificate", Value: cfg.ClientCertificate, Message: fmt.Sprintf("load client certificate/key: %v", err)})
		}
	}
	return errors.Join(errs...)
}

func buildTLSConfig(cfg Config) (*tls.Config, error) {
	if cfg.Certificate == "" && cfg.ClientCertificate == "" && cfg.ClientKey == "" {
		return nil, nil
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.Certificate != "" {
		pool, err := loadRootCAs(cfg.Certificate)
		if err != nil {
			return nil, fmt.Errorf("certificate: %w", err)
		}
		tlsCfg.RootCAs = pool
	}
	if cfg.ClientCertificate != "" && cfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCertificate, cfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("client certificate/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	return tlsCfg, nil
}

func loadRootCAs(path string) (*x509.CertPool, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("parse PEM certificates from %s", path)
	}
	return pool, nil
}
