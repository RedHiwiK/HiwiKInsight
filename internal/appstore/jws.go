// Package appstore verifies and decodes the signed payloads of App Store Server Notifications V2.
package appstore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Marker extensions Apple puts in its certificates, checked the same way as the official app-store-server-library
var (
	oidAppleLeaf         = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1}
	oidAppleIntermediate = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1}
)

// Verifier checks the x5c certificate chain (leaf → intermediate → Apple Root CA G3) and the ES256 signature.
type Verifier struct {
	roots *x509.CertPool
	now   func() time.Time
}

func NewVerifier() *Verifier {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(appleRootCAG3PEM)) {
		panic("appstore: invalid embedded Apple root CA")
	}
	return &Verifier{roots: pool, now: time.Now}
}

// Decode verifies the JWS and unmarshals its payload into out.
func (v *Verifier) Decode(signed string, out any) error {
	parts := strings.Split(signed, ".")
	if len(parts) != 3 {
		return errors.New("jws: malformed token")
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("jws: decode header: %w", err)
	}
	var header struct {
		Alg string   `json:"alg"`
		X5c []string `json:"x5c"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return fmt.Errorf("jws: parse header: %w", err)
	}
	if header.Alg != "ES256" {
		return fmt.Errorf("jws: unexpected alg %q", header.Alg)
	}
	if len(header.X5c) != 3 {
		return fmt.Errorf("jws: expected 3 certs in x5c, got %d", len(header.X5c))
	}

	certs := make([]*x509.Certificate, len(header.X5c))
	for i, s := range header.X5c {
		der, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return fmt.Errorf("jws: decode x5c[%d]: %w", i, err)
		}
		if certs[i], err = x509.ParseCertificate(der); err != nil {
			return fmt.Errorf("jws: parse x5c[%d]: %w", i, err)
		}
	}
	leaf, intermediate := certs[0], certs[1]
	if !hasExtension(leaf, oidAppleLeaf) || !hasExtension(intermediate, oidAppleIntermediate) {
		return errors.New("jws: certificate chain missing Apple marker extensions")
	}

	intermediates := x509.NewCertPool()
	intermediates.AddCert(intermediate)
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         v.roots,
		Intermediates: intermediates,
		CurrentTime:   v.now(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return fmt.Errorf("jws: verify chain: %w", err)
	}

	pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return errors.New("jws: leaf key is not ECDSA P-256")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return errors.New("jws: malformed signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return errors.New("jws: invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("jws: decode payload: %w", err)
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("jws: parse payload: %w", err)
	}
	return nil
}

func hasExtension(cert *x509.Certificate, oid asn1.ObjectIdentifier) bool {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oid) {
			return true
		}
	}
	return false
}
