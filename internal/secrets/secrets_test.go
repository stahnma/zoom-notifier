package secrets

import (
	"errors"
	"strings"
	"testing"
)

func mustCipher(t *testing.T) (*Cipher, string) {
	t.Helper()
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	c, err := New(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	return c, key
}

func TestRoundTrip(t *testing.T) {
	c, _ := mustCipher(t)
	sealed, err := c.Seal("xoxb-secret-token")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !strings.HasPrefix(sealed, Prefix) {
		t.Fatalf("sealed value missing prefix: %q", sealed)
	}
	if strings.Contains(sealed, "xoxb") {
		t.Fatalf("sealed value leaks plaintext: %q", sealed)
	}
	got, err := c.Open(sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got != "xoxb-secret-token" {
		t.Fatalf("got %q", got)
	}
}

func TestSealIsNonDeterministic(t *testing.T) {
	c, _ := mustCipher(t)
	a, _ := c.Seal("same")
	b, _ := c.Seal("same")
	if a == b {
		t.Fatal("two seals of the same plaintext should differ (random nonce)")
	}
}

func TestEmptyStringPassesThrough(t *testing.T) {
	c, _ := mustCipher(t)
	sealed, err := c.Seal("")
	if err != nil || sealed != "" {
		t.Fatalf("expected empty passthrough, got %q, %v", sealed, err)
	}
}

func TestLegacyPlaintextPassesThrough(t *testing.T) {
	c, _ := mustCipher(t)
	got, err := c.Open("plain-old-token")
	if err != nil || got != "plain-old-token" {
		t.Fatalf("expected legacy passthrough, got %q, %v", got, err)
	}
}

func TestNilCipherPassesThrough(t *testing.T) {
	var c *Cipher
	if c.Enabled() {
		t.Fatal("nil cipher should not be enabled")
	}
	sealed, err := c.Seal("token")
	if err != nil || sealed != "token" {
		t.Fatalf("nil seal: %q, %v", sealed, err)
	}
	got, err := c.Open("token")
	if err != nil || got != "token" {
		t.Fatalf("nil open: %q, %v", got, err)
	}
}

func TestNilCipherRejectsEncryptedValue(t *testing.T) {
	c, _ := mustCipher(t)
	sealed, _ := c.Seal("token")
	var none *Cipher
	if _, err := none.Open(sealed); !errors.Is(err, ErrNoKey) {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}
}

func TestWrongKeyFails(t *testing.T) {
	c1, _ := mustCipher(t)
	c2, _ := mustCipher(t)
	sealed, _ := c1.Seal("token")
	if _, err := c2.Open(sealed); err == nil {
		t.Fatal("expected decrypt failure with wrong key")
	}
}

func TestBadKeys(t *testing.T) {
	if _, err := New("not-hex"); err == nil {
		t.Fatal("expected error for non-hex key")
	}
	if _, err := New("abcd"); err == nil {
		t.Fatal("expected error for short key")
	}
	c, err := New("  ")
	if err != nil || c != nil {
		t.Fatalf("blank key should yield nil cipher, got %v, %v", c, err)
	}
}

func TestPtrHelpers(t *testing.T) {
	c, _ := mustCipher(t)
	if v, err := c.SealPtr(nil); v != nil || err != nil {
		t.Fatalf("SealPtr(nil) = %v, %v", v, err)
	}
	if v, err := c.OpenPtr(nil); v != nil || err != nil {
		t.Fatalf("OpenPtr(nil) = %v, %v", v, err)
	}
	in := "tok"
	sealed, err := c.SealPtr(&in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.OpenPtr(sealed)
	if err != nil || out == nil || *out != "tok" {
		t.Fatalf("OpenPtr roundtrip failed: %v, %v", out, err)
	}
}

func TestCorruptCiphertext(t *testing.T) {
	c, _ := mustCipher(t)
	if _, err := c.Open(Prefix + "!!!not-base64"); err == nil {
		t.Fatal("expected base64 error")
	}
	if _, err := c.Open(Prefix + "AAAA"); err == nil {
		t.Fatal("expected too-short error")
	}
}
