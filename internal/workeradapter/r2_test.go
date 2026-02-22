package workeradapter

import (
	"testing"
)

func TestBuildPublicObjectURL(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		bucket  string
		key     string
		want    string
		wantErr bool
	}{
		{
			name:   "simple",
			base:   "https://s3.example.com",
			bucket: "my-bucket",
			key:    "path/to/file.txt",
			want:   "https://s3.example.com/my-bucket/path/to/file.txt",
		},
		{
			name:   "with base path",
			base:   "https://s3.example.com/storage",
			bucket: "my-bucket",
			key:    "file.txt",
			want:   "https://s3.example.com/storage/my-bucket/file.txt",
		},
		{
			name:   "trailing slash in base",
			base:   "https://s3.example.com/",
			bucket: "bucket",
			key:    "key",
			want:   "https://s3.example.com/bucket/key",
		},
		{
			name:   "leading slash in key",
			base:   "https://s3.example.com",
			bucket: "bucket",
			key:    "/leading.txt",
			want:   "https://s3.example.com/bucket/leading.txt",
		},
		{
			name:   "special characters in key",
			base:   "https://s3.example.com",
			bucket: "bucket",
			key:    "path/file with spaces.txt",
			want:   "https://s3.example.com/bucket/path/file%20with%20spaces.txt",
		},
		{
			name:    "missing scheme",
			base:    "example.com",
			bucket:  "bucket",
			key:     "key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildPublicObjectURL(tt.base, tt.bucket, tt.key)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEscapePathSegments(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty", "", ""},
		{"simple", "a/b/c", "a/b/c"},
		{"spaces", "path/my file.txt", "path/my%20file.txt"},
		{"single segment", "file.txt", "file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapePathSegments(tt.path)
			if got != tt.want {
				t.Errorf("escapePathSegments(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestMinioR2Store_PublicURL(t *testing.T) {
	store := &MinioR2Store{
		BucketName:  "my-bucket",
		PublicS3URL: "https://s3.example.com",
	}

	u, err := store.PublicURL("path/to/file.txt")
	if err != nil {
		t.Fatalf("PublicURL: %v", err)
	}
	want := "https://s3.example.com/my-bucket/path/to/file.txt"
	if u != want {
		t.Errorf("PublicURL = %q, want %q", u, want)
	}
}

func TestMinioR2Store_PublicURL_NotConfigured(t *testing.T) {
	store := &MinioR2Store{
		BucketName:  "my-bucket",
		PublicS3URL: "",
	}

	_, err := store.PublicURL("file.txt")
	if err == nil {
		t.Error("expected error when PublicS3URL is empty")
	}
}
