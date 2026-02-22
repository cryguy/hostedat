package seaweedfs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7/pkg/signer"
)

// Client wraps the SeaweedFS IAM-compatible API for credential management.
// Bucket operations use standard S3 (minio-go); this client handles only IAM.
type Client struct {
	Endpoint   string // e.g. "http://127.0.0.1:8333"
	HTTPClient *http.Client

	// Credentials for SigV4-signed IAM requests. When set, all IAM
	// requests are signed with AWS Signature V4 using service "s3"
	// (SeaweedFS shares its S3 auth layer with the IAM endpoint).
	AccessKeyID     string
	SecretAccessKey string
	Region          string
}

// NewClient creates an unauthenticated SeaweedFS IAM client.
// Use NewClientWithAuth for managed instances that require SigV4 signing.
func NewClient(endpoint string) *Client {
	return &Client{
		Endpoint: endpoint,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NewClientWithAuth creates a SeaweedFS IAM client that signs requests
// with AWS Signature V4 using the provided S3 admin credentials.
func NewClientWithAuth(endpoint, accessKey, secretKey, region string) *Client {
	return &Client{
		Endpoint:        endpoint,
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		Region:          region,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// AccessKeyResult holds the credentials returned by CreateAccessKey.
type AccessKeyResult struct {
	AccessKeyID     string
	SecretAccessKey string
	UserName        string
}

// createAccessKeyResponse is the XML envelope for IAM CreateAccessKey responses.
type createAccessKeyResponse struct {
	XMLName xml.Name `xml:"CreateAccessKeyResponse"`
	Result  struct {
		AccessKey struct {
			AccessKeyId     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			UserName        string `xml:"UserName"`
		} `xml:"AccessKey"`
	} `xml:"CreateAccessKeyResult"`
}

func (c *Client) doIAM(params url.Values) ([]byte, error) {
	payload := []byte(params.Encode())
	// Ensure the URL has a path ("/") so SigV4 canonical URI is correct.
	endpoint := c.Endpoint
	if !strings.HasSuffix(endpoint, "/") {
		endpoint += "/"
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("building IAM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ContentLength = int64(len(payload))

	// Sign the request if credentials are configured.
	if c.AccessKeyID != "" && c.SecretAccessKey != "" {
		// Pre-compute the payload hash so SignV4 doesn't consume the body reader.
		h := sha256.Sum256(payload)
		req.Header.Set("X-Amz-Content-Sha256", hex.EncodeToString(h[:]))
		req = signer.SignV4(*req, c.AccessKeyID, c.SecretAccessKey, "", c.Region)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IAM request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading IAM response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("IAM %s returned %d: %s", params.Get("Action"), resp.StatusCode, string(body))
	}

	return body, nil
}

// CreateUser creates an IAM user.
func (c *Client) CreateUser(userName string) error {
	_, err := c.doIAM(url.Values{
		"Action":   {"CreateUser"},
		"UserName": {userName},
	})
	return err
}

// DeleteUser deletes an IAM user (and its access keys).
func (c *Client) DeleteUser(userName string) error {
	_, err := c.doIAM(url.Values{
		"Action":   {"DeleteUser"},
		"UserName": {userName},
	})
	return err
}

// CreateAccessKey creates an access key for an IAM user.
func (c *Client) CreateAccessKey(userName string) (*AccessKeyResult, error) {
	data, err := c.doIAM(url.Values{
		"Action":   {"CreateAccessKey"},
		"UserName": {userName},
	})
	if err != nil {
		return nil, err
	}

	var resp createAccessKeyResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing CreateAccessKey response: %w", err)
	}

	return &AccessKeyResult{
		AccessKeyID:     resp.Result.AccessKey.AccessKeyId,
		SecretAccessKey: resp.Result.AccessKey.SecretAccessKey,
		UserName:        resp.Result.AccessKey.UserName,
	}, nil
}

// DeleteAccessKey deletes an access key by ID.
func (c *Client) DeleteAccessKey(accessKeyID string) error {
	_, err := c.doIAM(url.Values{
		"Action":      {"DeleteAccessKey"},
		"AccessKeyId": {accessKeyID},
	})
	return err
}

// PutUserPolicy attaches an inline policy to an IAM user.
func (c *Client) PutUserPolicy(userName, policyName, policyJSON string) error {
	_, err := c.doIAM(url.Values{
		"Action":         {"PutUserPolicy"},
		"UserName":       {userName},
		"PolicyName":     {policyName},
		"PolicyDocument": {policyJSON},
	})
	return err
}

// Health checks if the SeaweedFS S3 endpoint is reachable.
// Any HTTP response (even 403 when auth is enabled) means the server is up.
func (c *Client) Health() error {
	resp, err := c.HTTPClient.Get(c.Endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}
