package s3

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"time"
)

// S3 XML response types matching the real S3 schema.

// ErrorResponse is the standard S3 error XML.
type ErrorResponse struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource,omitempty"`
	RequestID string   `xml:"RequestId"`
}

// ListBucketResult is the response for ListObjectsV2.
type ListBucketResult struct {
	XMLName               xml.Name       `xml:"ListBucketResult"`
	XMLNS                 string         `xml:"xmlns,attr"`
	Name                  string         `xml:"Name"`
	Prefix                string         `xml:"Prefix"`
	Delimiter             string         `xml:"Delimiter,omitempty"`
	MaxKeys               int            `xml:"MaxKeys"`
	KeyCount              int            `xml:"KeyCount"`
	IsTruncated           bool           `xml:"IsTruncated"`
	ContinuationToken     string         `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string         `xml:"NextContinuationToken,omitempty"`
	StartAfter            string         `xml:"StartAfter,omitempty"`
	Contents              []ListObject   `xml:"Contents,omitempty"`
	CommonPrefixes        []CommonPrefix `xml:"CommonPrefixes,omitempty"`
}

// ListObject represents a single object in a ListObjectsV2 response.
type ListObject struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

// CommonPrefix represents a common prefix in a ListObjectsV2 response.
type CommonPrefix struct {
	Prefix string `xml:"Prefix"`
}

// CopyObjectResult is the response for CopyObject.
type CopyObjectResult struct {
	XMLName      xml.Name `xml:"CopyObjectResult"`
	LastModified string   `xml:"LastModified"`
	ETag         string   `xml:"ETag"`
}

// DeleteRequest is the XML body for DeleteObjects batch.
type DeleteRequest struct {
	XMLName xml.Name       `xml:"Delete"`
	Quiet   bool           `xml:"Quiet"`
	Objects []DeleteObject `xml:"Object"`
}

// DeleteObject represents a single object in a batch delete request.
type DeleteObject struct {
	Key string `xml:"Key"`
}

// DeleteResult is the response for DeleteObjects.
type DeleteResult struct {
	XMLName xml.Name        `xml:"DeleteResult"`
	XMLNS   string          `xml:"xmlns,attr"`
	Deleted []DeletedObject `xml:"Deleted,omitempty"`
	Errors  []DeleteError   `xml:"Error,omitempty"`
}

// DeletedObject represents a successfully deleted object.
type DeletedObject struct {
	Key string `xml:"Key"`
}

// DeleteError represents a failed deletion.
type DeleteError struct {
	Key     string `xml:"Key"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// InitiateMultipartUploadResult is returned by CreateMultipartUpload.
type InitiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	XMLNS    string   `xml:"xmlns,attr"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadId string   `xml:"UploadId"`
}

// CompleteMultipartUploadRequest is the XML body for CompleteMultipartUpload.
type CompleteMultipartUploadRequest struct {
	XMLName xml.Name       `xml:"CompleteMultipartUpload"`
	Parts   []CompletePart `xml:"Part"`
}

// CompletePart represents a single part in a CompleteMultipartUpload request.
type CompletePart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

// CompleteMultipartUploadResult is returned by CompleteMultipartUpload.
type CompleteMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	XMLNS    string   `xml:"xmlns,attr"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

// WriteErrorResponse writes an S3-format XML error response.
func WriteErrorResponse(w http.ResponseWriter, httpStatus int, code, message, resource string) {
	resp := ErrorResponse{
		Code:      code,
		Message:   message,
		Resource:  resource,
		RequestID: "hostedat",
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(httpStatus)
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(resp)
}

// WriteXMLResponse writes an XML response with the given status code.
func WriteXMLResponse(w http.ResponseWriter, httpStatus int, v interface{}) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(httpStatus)
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(v)
}

// FormatS3Time formats a time.Time in S3's ISO 8601 format.
func FormatS3Time(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// FormatHTTPTime formats a time.Time for HTTP Last-Modified header.
func FormatHTTPTime(t time.Time) string {
	return t.UTC().Format(http.TimeFormat)
}

// QuoteETag ensures an ETag is wrapped in quotes.
func QuoteETag(etag string) string {
	if len(etag) >= 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
		return etag
	}
	return fmt.Sprintf(`"%s"`, etag)
}

// UnquoteETag removes surrounding quotes from an ETag.
func UnquoteETag(etag string) string {
	if len(etag) >= 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
		return etag[1 : len(etag)-1]
	}
	return etag
}
