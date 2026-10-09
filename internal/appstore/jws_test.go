package appstore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

type testChain struct {
	root, inter, leaf *x509.Certificate
	leafKey           *ecdsa.PrivateKey
}

func newTestChain(t *testing.T, markers bool) testChain {
	t.Helper()
	mk := func(cn string, isCA bool, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, ext *pkixExt) (*x509.Certificate, *ecdsa.PrivateKey) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(time.Now().UnixNano()),
			Subject:               pkix.Name{CommonName: cn},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(time.Hour),
			IsCA:                  isCA,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		}
		if ext != nil && markers {
			tmpl.ExtraExtensions = []pkix.Extension{{Id: ext.oid, Value: []byte{0x05, 0x00}}}
		}
		if parent == nil {
			parent, parentKey = tmpl, key
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
		if err != nil {
			t.Fatal(err)
		}
		cert, _ := x509.ParseCertificate(der)
		return cert, key
	}
	root, rootKey := mk("root", true, nil, nil, nil)
	inter, interKey := mk("inter", true, root, rootKey, &pkixExt{oidAppleIntermediate})
	leaf, leafKey := mk("leaf", false, inter, interKey, &pkixExt{oidAppleLeaf})
	return testChain{root, inter, leaf, leafKey}
}

type pkixExt struct{ oid []int }

func (c testChain) sign(t *testing.T, payload any) string {
	t.Helper()
	x5c := []string{}
	for _, cert := range []*x509.Certificate{c.leaf, c.inter, c.root} {
		x5c = append(x5c, base64.StdEncoding.EncodeToString(cert.Raw))
	}
	h, _ := json.Marshal(map[string]any{"alg": "ES256", "x5c": x5c})
	p, _ := json.Marshal(payload)
	input := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	digest := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, c.leafKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func verifierFor(root *x509.Certificate) *Verifier {
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return &Verifier{roots: pool, now: time.Now}
}

func TestDecodeValid(t *testing.T) {
	c := newTestChain(t, true)
	token := c.sign(t, map[string]any{"notificationType": "ONE_TIME_CHARGE", "data": map[string]any{"bundleId": "a.b"}})

	var p NotificationPayload
	if err := verifierFor(c.root).Decode(token, &p); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if p.NotificationType != "ONE_TIME_CHARGE" || p.Data.BundleID != "a.b" {
		t.Fatalf("unexpected payload %+v", p)
	}
}

func TestDecodeRejects(t *testing.T) {
	c := newTestChain(t, true)
	token := c.sign(t, map[string]any{"notificationType": "ONE_TIME_CHARGE"})
	noMarkers := newTestChain(t, false)
	parts := strings.Split(token, ".")
	forged := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"notificationType":"REFUND"}`)) + "." + parts[2]

	cases := map[string]struct {
		v     *Verifier
		token string
	}{
		"tampered payload": {verifierFor(c.root), forged},
		"untrusted root":   {NewVerifier(), token},
		"missing markers":  {verifierFor(noMarkers.root), noMarkers.sign(t, map[string]any{"notificationType": "TEST"})},
		"malformed":        {verifierFor(c.root), "a.b"},
	}
	for name, tc := range cases {
		var p NotificationPayload
		if err := tc.v.Decode(tc.token, &p); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
