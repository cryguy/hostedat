package s3

import (
	"crypto/hmac"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// signRequest signs an HTTP request with SigV4 for testing.
func signRequest(r *http.Request, accessKeyID, secretKey, region, service string, body []byte) {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	r.Header.Set("X-Amz-Date", amzDate)
	r.Header.Set("Host", r.Host)

	payloadHash := hashSHA256(body)
	r.Header.Set("X-Amz-Content-Sha256", payloadHash)

	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	if r.Header.Get("Content-Type") != "" {
		signedHeaders = append(signedHeaders, "content-type")
	}

	canonicalRequest := buildCanonicalRequest(r, signedHeaders, body)
	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hashSHA256([]byte(canonicalRequest))

	signingKey := deriveSigningKey(secretKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := "AWS4-HMAC-SHA256 " +
		"Credential=" + accessKeyID + "/" + scope + ", " +
		"SignedHeaders=" + strings.Join(signedHeaders, ";") + ", " +
		"Signature=" + signature
	r.Header.Set("Authorization", authHeader)
}

func TestParseAuthorization_Valid(t *testing.T) {
	header := "AWS4-HMAC-SHA256 Credential=AKID/20240101/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=abcdef1234567890"
	auth, err := ParseAuthorization(header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth.AccessKeyID != "AKID" {
		t.Errorf("AccessKeyID = %q, want AKID", auth.AccessKeyID)
	}
	if auth.Region != "us-east-1" {
		t.Errorf("Region = %q, want us-east-1", auth.Region)
	}
	if auth.Service != "s3" {
		t.Errorf("Service = %q, want s3", auth.Service)
	}
	if auth.Date != "20240101" {
		t.Errorf("Date = %q, want 20240101", auth.Date)
	}
	if len(auth.SignedHeaders) != 2 {
		t.Errorf("SignedHeaders count = %d, want 2", len(auth.SignedHeaders))
	}
	if auth.Signature != "abcdef1234567890" {
		t.Errorf("Signature = %q, want abcdef1234567890", auth.Signature)
	}
}

func TestParseAuthorization_InvalidScheme(t *testing.T) {
	_, err := ParseAuthorization("Basic dXNlcjpwYXNz")
	if err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
}

func TestParseAuthorization_Incomplete(t *testing.T) {
	_, err := ParseAuthorization("AWS4-HMAC-SHA256 Credential=AKID/20240101/us-east-1/s3/aws4_request")
	if err == nil {
		t.Fatal("expected error for incomplete header")
	}
}

func TestVerifySignature_Valid(t *testing.T) {
	body := []byte("test body content")
	req := httptest.NewRequest(http.MethodPut, "http://localhost/bucket/key", strings.NewReader(string(body)))
	req.Host = "localhost"

	accessKeyID := "AKIDTEST12345678"
	secretKey := "secretkey1234567890abcdef"

	signRequest(req, accessKeyID, secretKey, "us-east-1", "s3", body)

	auth, err := ParseAuthorization(req.Header.Get("Authorization"))
	if err != nil {
		t.Fatalf("parse auth: %v", err)
	}

	err = VerifySignature(req, auth, secretKey, body)
	if err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}
}

func TestVerifySignature_WrongKey(t *testing.T) {
	body := []byte("test body")
	req := httptest.NewRequest(http.MethodPut, "http://localhost/bucket/key", strings.NewReader(string(body)))
	req.Host = "localhost"

	signRequest(req, "AKID", "correctsecret", "us-east-1", "s3", body)

	auth, err := ParseAuthorization(req.Header.Get("Authorization"))
	if err != nil {
		t.Fatalf("parse auth: %v", err)
	}

	err = VerifySignature(req, auth, "wrongsecret", body)
	if err == nil {
		t.Fatal("expected signature mismatch error")
	}
	if !strings.Contains(err.Error(), "signature does not match") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestVerifySignature_ExpiredTimestamp(t *testing.T) {
	body := []byte("test body")
	req := httptest.NewRequest(http.MethodGet, "http://localhost/bucket/key", nil)
	req.Host = "localhost"

	// Set a timestamp 20 minutes in the past.
	oldTime := time.Now().UTC().Add(-20 * time.Minute)
	dateStamp := oldTime.Format("20060102")
	amzDate := oldTime.Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", hashSHA256(body))

	secretKey := "secret"
	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	canonicalRequest := buildCanonicalRequest(req, signedHeaders, body)
	scope := dateStamp + "/us-east-1/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hashSHA256([]byte(canonicalRequest))
	signingKey := deriveSigningKey(secretKey, dateStamp, "us-east-1", "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := "AWS4-HMAC-SHA256 Credential=AKID/" + scope + ", SignedHeaders=" + strings.Join(signedHeaders, ";") + ", Signature=" + signature
	req.Header.Set("Authorization", authHeader)

	auth, _ := ParseAuthorization(authHeader)
	err := VerifySignature(req, auth, secretKey, body)
	if err == nil {
		t.Fatal("expected timestamp expired error")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDeriveSigningKey(t *testing.T) {
	// Verify that deriveSigningKey produces deterministic output.
	key1 := deriveSigningKey("secret", "20240101", "us-east-1", "s3")
	key2 := deriveSigningKey("secret", "20240101", "us-east-1", "s3")
	if !hmac.Equal(key1, key2) {
		t.Error("signing keys should be deterministic")
	}

	// Different secret should produce different key.
	key3 := deriveSigningKey("other", "20240101", "us-east-1", "s3")
	if hmac.Equal(key1, key3) {
		t.Error("different secrets should produce different keys")
	}
}

func TestHashSHA256(t *testing.T) {
	hash := hashSHA256([]byte(""))
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if hash != expected {
		t.Errorf("empty hash = %q, want %q", hash, expected)
	}
}

func TestBuildCanonicalQueryString(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{"empty", "", ""},
		{"single", "prefix=test", "prefix=test"},
		{"sorted", "z=1&a=2", "a=2&z=1"},
		{"encoded", "prefix=hello%20world", "prefix=hello%20world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://localhost/?"+tt.query, nil)
			result := buildCanonicalQueryString(req.URL.Query())
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}
