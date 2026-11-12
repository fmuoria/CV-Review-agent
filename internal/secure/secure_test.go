package secure

import (
	"testing"
	"time"
)

func TestEncryptRoundTrip(t *testing.T) {
	b := New("a-very-long-secret-for-testing-purposes-123")
	c, err := b.Encrypt("refresh-token")
	if err != nil {
		t.Fatal(err)
	}
	if p, err := b.Decrypt(c); err != nil || p != "refresh-token" {
		t.Fatalf("decrypt = %q, %v", p, err)
	}
	if _, err := New("another-secret-of-similar-length-000000").Decrypt(c); err == nil {
		t.Fatal("decrypt with the wrong secret should fail")
	}
}

func TestSignVerify(t *testing.T) {
	b := New("secret-secret-secret-secret-secret-1")
	s := b.Sign("login|abc|/jobs", time.Minute)
	if v, err := b.Verify(s); err != nil || v != "login|abc|/jobs" {
		t.Fatalf("verify = %q, %v", v, err)
	}
	if _, err := b.Verify(s + "x"); err == nil {
		t.Fatal("tampered value accepted")
	}
	if _, err := b.Verify(b.Sign("x", -time.Second)); err == nil {
		t.Fatal("expired value accepted")
	}
}
