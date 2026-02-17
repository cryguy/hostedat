package seaweedfs

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client wraps the SeaweedFS IAM-compatible API for credential management.
// Bucket operations use standard S3 (minio-go); this client handles only IAM.
type Client struct {
	Endpoint   string // e.g. "http://127.0.0.1:8333"
	HTTPClient *http.Client
}

// NewClient creates a SeaweedFS IAM client.
func NewClient(endpoint string) *Client {
	return &Client{
		Endpoint: endpoint,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// AccessKeyResult holds the credentials returned by CreateAccessKey.
type AccessKeyResult struct {
	AccessKeyID    string
	SecretAccessKey string
	UserName       string
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
	resp, err := c.HTTPClient.PostForm(c.Endpoint, params)
	if err != nil {
		return nil, fmt.Errorf("IAM request: %w", err)
	}
	defer resp.Body.Close()

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
		AccessKeyID:    resp.Result.AccessKey.AccessKeyId,
		SecretAccessKey: resp.Result.AccessKey.SecretAccessKey,
		UserName:       resp.Result.AccessKey.UserName,
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
func (c *Client) Health() error {
	resp, err := c.HTTPClient.Get(c.Endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SeaweedFS health check returned %d", resp.StatusCode)
	}
	return nil
}
