package auth

import "testing"

func TestGenerateCodeVerifier(t *testing.T) {
	v, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier: %v", err)
	}
	// 32 bytes base64url-encoded without padding = 43 chars
	if len(v) != 43 {
		t.Errorf("verifier length = %d, want 43", len(v))
	}

	// Should be unique
	v2, _ := GenerateCodeVerifier()
	if v == v2 {
		t.Error("two verifiers should not be equal")
	}
}

func TestComputeCodeChallenge_Deterministic(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	c1 := ComputeCodeChallenge(verifier)
	c2 := ComputeCodeChallenge(verifier)
	if c1 != c2 {
		t.Errorf("challenge not deterministic: %q != %q", c1, c2)
	}
	if c1 == "" {
		t.Error("challenge should not be empty")
	}
}

func TestVerifyCodeChallenge(t *testing.T) {
	verifier, _ := GenerateCodeVerifier()
	challenge := ComputeCodeChallenge(verifier)

	if !VerifyCodeChallenge(verifier, challenge) {
		t.Error("VerifyCodeChallenge should return true for correct verifier")
	}
	if VerifyCodeChallenge("wrong-verifier", challenge) {
		t.Error("VerifyCodeChallenge should return false for wrong verifier")
	}
}

func TestGenerateAuthCode(t *testing.T) {
	code, err := GenerateAuthCode()
	if err != nil {
		t.Fatalf("GenerateAuthCode: %v", err)
	}
	// 32 bytes hex-encoded = 64 chars
	if len(code) != 64 {
		t.Errorf("auth code length = %d, want 64", len(code))
	}

	// Should be unique
	code2, _ := GenerateAuthCode()
	if code == code2 {
		t.Error("two auth codes should not be equal")
	}
}

func TestGenerateCodeVerifier_URLSafe(t *testing.T) {
	v, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier: %v", err)
	}
	// base64url should not contain +, /, or =
	for _, ch := range v {
		if ch == '+' || ch == '/' || ch == '=' {
			t.Errorf("verifier contains non-URL-safe character: %q in %q", ch, v)
		}
	}
}
