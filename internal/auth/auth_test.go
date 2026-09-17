package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestNewTokenFormat(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if !strings.HasPrefix(token, "homey_") {
		t.Fatalf("token = %q, want the homey_ prefix", token)
	}
	if len(token) != len("homey_")+43 {
		t.Fatalf("token length = %d, want %d (32 bytes in base64url)", len(token), len("homey_")+43)
	}

	other, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if token == other {
		t.Fatal("two generated tokens are identical")
	}
}

func TestHashAndVerify(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	hash := Hash(token)
	if len(hash) != 64 { // hex-encoded SHA-256
		t.Fatalf("hash = %q, want 64 hex characters", hash)
	}
	if Hash(token) != hash {
		t.Fatal("hashing the same token twice produced different hashes")
	}
	if !Verify(token, hash) {
		t.Fatal("Verify(token, hash) = false")
	}
	if Verify("homey_wrong", hash) {
		t.Fatal("Verify accepted a different token")
	}
}

func TestParseScopes(t *testing.T) {
	scopes, err := ParseScopes("read,write")
	if err != nil {
		t.Fatalf("ParseScopes: %v", err)
	}
	if !scopes.Contains(ScopeRead) || !scopes.Contains(ScopeWrite) || !scopes.CanWrite() {
		t.Fatalf("scopes = %v", scopes)
	}

	scopes, err = ParseScopes("  WRITE , read , write ")
	if err != nil {
		t.Fatalf("ParseScopes: %v", err)
	}
	if got, want := scopes.String(), "write,read"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	for _, raw := range []string{"", "  ", "admin", "read,bogus"} {
		if _, err := ParseScopes(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseScopes(%q) = %v, want ErrInvalid", raw, err)
		}
	}
}

func TestValidate(t *testing.T) {
	scopes, err := ParseScopes("read,write")
	if err != nil {
		t.Fatalf("ParseScopes: %v", err)
	}
	if err := Validate("UI", scopes, ConfirmationRequired); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, tc := range []struct {
		name   string
		scopes Scopes
		policy Confirmation
	}{
		{"", Scopes{ScopeRead}, ConfirmationRequired},
		{"UI", nil, ConfirmationRequired},
		{"UI", Scopes{ScopeWrite}, Confirmation("whatever")},
	} {
		if err := Validate(tc.name, tc.scopes, tc.policy); !errors.Is(err, ErrInvalid) {
			t.Errorf("Validate(%q, %v, %v) = %v, want ErrInvalid", tc.name, tc.scopes, tc.policy, err)
		}
	}
}
