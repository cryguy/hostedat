package s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// SigV4Auth holds the parsed components of an AWS Signature V4 Authorization header.
type SigV4Auth struct {
	AccessKeyID   string
	Region        string
	Service       string
	Date          string // YYYYMMDD
	SignedHeaders []string
	Signature     string
}

// ParseAuthorization parses an AWS4-HMAC-SHA256 Authorization header.
func ParseAuthorization(header string) (*SigV4Auth, error) {
	if !strings.HasPrefix(header, "AWS4-HMAC-SHA256 ") {
		return nil, fmt.Errorf("unsupported authorization scheme")
	}

	parts := header[len("AWS4-HMAC-SHA256 "):]
	auth := &SigV4Auth{}

	for _, part := range strings.Split(parts, ", ") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "Credential=") {
			cred := strings.TrimPrefix(part, "Credential=")
			credParts := strings.SplitN(cred, "/", 5)
			if len(credParts) != 5 {
				return nil, fmt.Errorf("invalid credential format")
			}
			auth.AccessKeyID = credParts[0]
			auth.Date = credParts[1]
			auth.Region = credParts[2]
			auth.Service = credParts[3]
			// credParts[4] should be "aws4_request"
		} else if strings.HasPrefix(part, "SignedHeaders=") {
			signed := strings.TrimPrefix(part, "SignedHeaders=")
			auth.SignedHeaders = strings.Split(signed, ";")
		} else if strings.HasPrefix(part, "Signature=") {
			auth.Signature = strings.TrimPrefix(part, "Signature=")
		}
	}

	if auth.AccessKeyID == "" || auth.Signature == "" || len(auth.SignedHeaders) == 0 {
		return nil, fmt.Errorf("incomplete authorization header")
	}

	return auth, nil
}

// VerifySignature verifies the AWS Signature V4 for the given request.
func VerifySignature(r *http.Request, auth *SigV4Auth, secretKey string, body []byte) error {
	// Parse the X-Amz-Date header.
	amzDate := r.Header.Get("X-Amz-Date")
	if amzDate == "" {
		return fmt.Errorf("missing X-Amz-Date header")
	}

	// Verify the date is not too old (15 minute window).
	reqTime, err := time.Parse("20060102T150405Z", amzDate)
	if err != nil {
		return fmt.Errorf("invalid X-Amz-Date format")
	}
	if time.Since(reqTime).Abs() > 15*time.Minute {
		return fmt.Errorf("request timestamp expired")
	}

	// Verify date component matches.
	dateStamp := amzDate[:8]
	if dateStamp != auth.Date {
		return fmt.Errorf("date mismatch between credential and X-Amz-Date")
	}

	// Step 1: Create canonical request.
	canonicalRequest := buildCanonicalRequest(r, auth.SignedHeaders, body)

	// Step 2: Create string to sign.
	scope := dateStamp + "/" + auth.Region + "/" + auth.Service + "/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" +
		amzDate + "\n" +
		scope + "\n" +
		hashSHA256([]byte(canonicalRequest))

	// Step 3: Calculate signature.
	signingKey := deriveSigningKey(secretKey, dateStamp, auth.Region, auth.Service)
	expectedSig := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	if !hmac.Equal([]byte(expectedSig), []byte(auth.Signature)) {
		return fmt.Errorf("signature does not match")
	}

	return nil
}

func buildCanonicalRequest(r *http.Request, signedHeaders []string, body []byte) string {
	// Canonical URI.
	canonicalURI := r.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	// URI-encode each path segment.
	segments := strings.Split(canonicalURI, "/")
	for i, seg := range segments {
		segments[i] = uriEncode(seg, false)
	}
	canonicalURI = strings.Join(segments, "/")

	// Canonical query string.
	canonicalQueryString := buildCanonicalQueryString(r.URL.Query())

	// Canonical headers.
	sort.Strings(signedHeaders)
	var canonicalHeaders strings.Builder
	for _, h := range signedHeaders {
		h = strings.ToLower(h)
		vals := r.Header.Values(http.CanonicalHeaderKey(h))
		if h == "host" {
			vals = []string{r.Host}
		}
		for i, v := range vals {
			vals[i] = strings.TrimSpace(v)
		}
		canonicalHeaders.WriteString(h + ":" + strings.Join(vals, ",") + "\n")
	}

	signedHeadersStr := strings.Join(signedHeaders, ";")

	// Payload hash.
	payloadHash := r.Header.Get("X-Amz-Content-Sha256")
	if payloadHash == "" {
		payloadHash = hashSHA256(body)
	}

	return r.Method + "\n" +
		canonicalURI + "\n" +
		canonicalQueryString + "\n" +
		canonicalHeaders.String() + "\n" +
		signedHeadersStr + "\n" +
		payloadHash
}

func buildCanonicalQueryString(query url.Values) string {
	if len(query) == 0 {
		return ""
	}

	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		vals := query[k]
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}

	return strings.Join(parts, "&")
}

func uriEncode(s string, encodeSlash bool) string {
	var buf strings.Builder
	for _, b := range []byte(s) {
		if isUnreserved(b) {
			buf.WriteByte(b)
		} else if b == '/' && !encodeSlash {
			buf.WriteByte('/')
		} else {
			buf.WriteString(fmt.Sprintf("%%%02X", b))
		}
	}
	return buf.String()
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~'
}

func hashSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func deriveSigningKey(secretKey, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}
