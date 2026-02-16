package s3

import (
	"encoding/xml"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWriteErrorResponse_ValidXML(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErrorResponse(w, 404, "NoSuchKey", "The specified key does not exist.", "/bucket/key")

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/xml" {
		t.Errorf("Content-Type = %q, want application/xml", ct)
	}

	// Parse XML to verify it's valid.
	var errResp ErrorResponse
	if err := xml.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("invalid XML: %v\nbody: %s", err, w.Body.String())
	}
	if errResp.Code != "NoSuchKey" {
		t.Errorf("Code = %q, want NoSuchKey", errResp.Code)
	}
	if errResp.Message != "The specified key does not exist." {
		t.Errorf("Message = %q", errResp.Message)
	}
	if errResp.Resource != "/bucket/key" {
		t.Errorf("Resource = %q, want /bucket/key", errResp.Resource)
	}
}

func TestWriteErrorResponse_SignatureDoesNotMatch(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErrorResponse(w, 403, "SignatureDoesNotMatch", "The request signature we calculated does not match.", "/")

	var errResp ErrorResponse
	if err := xml.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if errResp.Code != "SignatureDoesNotMatch" {
		t.Errorf("Code = %q", errResp.Code)
	}
}

func TestWriteErrorResponse_InvalidAccessKeyId(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErrorResponse(w, 403, "InvalidAccessKeyId", "The AWS Access Key Id does not exist.", "/")

	var errResp ErrorResponse
	if err := xml.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if errResp.Code != "InvalidAccessKeyId" {
		t.Errorf("Code = %q", errResp.Code)
	}
}

func TestWriteXMLResponse_ListBucketResult(t *testing.T) {
	w := httptest.NewRecorder()
	result := ListBucketResult{
		XMLNS:       "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:        "my-bucket",
		Prefix:      "photos/",
		MaxKeys:     1000,
		KeyCount:    2,
		IsTruncated: false,
		Contents: []ListObject{
			{Key: "photos/a.jpg", LastModified: "2024-01-01T00:00:00Z", ETag: `"abc123"`, Size: 1024, StorageClass: "STANDARD"},
			{Key: "photos/b.jpg", LastModified: "2024-01-02T00:00:00Z", ETag: `"def456"`, Size: 2048, StorageClass: "STANDARD"},
		},
	}
	WriteXMLResponse(w, 200, result)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// Verify valid XML.
	var parsed ListBucketResult
	if err := xml.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid XML: %v\nbody: %s", err, w.Body.String())
	}
	if parsed.Name != "my-bucket" {
		t.Errorf("Name = %q", parsed.Name)
	}
	if parsed.Prefix != "photos/" {
		t.Errorf("Prefix = %q", parsed.Prefix)
	}
	if parsed.KeyCount != 2 {
		t.Errorf("KeyCount = %d, want 2", parsed.KeyCount)
	}
	if len(parsed.Contents) != 2 {
		t.Errorf("Contents length = %d, want 2", len(parsed.Contents))
	}
}

func TestWriteXMLResponse_CommonPrefixes(t *testing.T) {
	w := httptest.NewRecorder()
	result := ListBucketResult{
		XMLNS:       "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:        "my-bucket",
		Delimiter:   "/",
		MaxKeys:     1000,
		KeyCount:    1,
		IsTruncated: false,
		CommonPrefixes: []CommonPrefix{
			{Prefix: "photos/"},
			{Prefix: "videos/"},
		},
	}
	WriteXMLResponse(w, 200, result)

	var parsed ListBucketResult
	if err := xml.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if len(parsed.CommonPrefixes) != 2 {
		t.Errorf("CommonPrefixes length = %d, want 2", len(parsed.CommonPrefixes))
	}
}

func TestWriteXMLResponse_DeleteResult(t *testing.T) {
	w := httptest.NewRecorder()
	result := DeleteResult{
		XMLNS: "http://s3.amazonaws.com/doc/2006-03-01/",
		Deleted: []DeletedObject{
			{Key: "file1.txt"},
			{Key: "file2.txt"},
		},
	}
	WriteXMLResponse(w, 200, result)

	var parsed DeleteResult
	if err := xml.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if len(parsed.Deleted) != 2 {
		t.Errorf("Deleted length = %d, want 2", len(parsed.Deleted))
	}
}

func TestWriteXMLResponse_CopyObjectResult(t *testing.T) {
	w := httptest.NewRecorder()
	result := CopyObjectResult{
		LastModified: FormatS3Time(time.Now()),
		ETag:         `"abc123"`,
	}
	WriteXMLResponse(w, 200, result)

	var parsed CopyObjectResult
	if err := xml.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if parsed.ETag != `"abc123"` {
		t.Errorf("ETag = %q", parsed.ETag)
	}
}

func TestFormatS3Time(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	formatted := FormatS3Time(ts)
	if !strings.Contains(formatted, "2024-01-15") {
		t.Errorf("formatted time = %q, expected to contain 2024-01-15", formatted)
	}
}

func TestQuoteETag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abc123", `"abc123"`},
		{`"abc123"`, `"abc123"`},
		{"", `""`},
	}
	for _, tt := range tests {
		result := QuoteETag(tt.input)
		if result != tt.expected {
			t.Errorf("QuoteETag(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestUnquoteETag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"abc123"`, "abc123"},
		{"abc123", "abc123"},
		{`""`, ""},
	}
	for _, tt := range tests {
		result := UnquoteETag(tt.input)
		if result != tt.expected {
			t.Errorf("UnquoteETag(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestDeleteRequest_ParseXML(t *testing.T) {
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<Delete>
  <Quiet>true</Quiet>
  <Object><Key>file1.txt</Key></Object>
  <Object><Key>file2.txt</Key></Object>
</Delete>`

	var req DeleteRequest
	if err := xml.Unmarshal([]byte(xmlBody), &req); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !req.Quiet {
		t.Error("Quiet should be true")
	}
	if len(req.Objects) != 2 {
		t.Errorf("Objects length = %d, want 2", len(req.Objects))
	}
	if req.Objects[0].Key != "file1.txt" {
		t.Errorf("first key = %q", req.Objects[0].Key)
	}
}

func TestCompleteMultipartUploadRequest_ParseXML(t *testing.T) {
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUpload>
  <Part><PartNumber>1</PartNumber><ETag>"abc"</ETag></Part>
  <Part><PartNumber>2</PartNumber><ETag>"def"</ETag></Part>
</CompleteMultipartUpload>`

	var req CompleteMultipartUploadRequest
	if err := xml.Unmarshal([]byte(xmlBody), &req); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(req.Parts) != 2 {
		t.Errorf("Parts length = %d, want 2", len(req.Parts))
	}
	if req.Parts[0].PartNumber != 1 {
		t.Errorf("first part number = %d", req.Parts[0].PartNumber)
	}
	if req.Parts[1].ETag != `"def"` {
		t.Errorf("second part ETag = %q", req.Parts[1].ETag)
	}
}
