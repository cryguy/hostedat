package seaweedfs

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mockSeaweedFS(t *testing.T) (*httptest.Server, *Client) {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		action := r.FormValue("Action")
		switch action {
		case "CreateUser":
			if r.FormValue("UserName") == "" {
				http.Error(w, "missing UserName", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<CreateUserResponse/>"))

		case "DeleteUser":
			if r.FormValue("UserName") == "" {
				http.Error(w, "missing UserName", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<DeleteUserResponse/>"))

		case "CreateAccessKey":
			userName := r.FormValue("UserName")
			if userName == "" {
				http.Error(w, "missing UserName", http.StatusBadRequest)
				return
			}
			resp := createAccessKeyResponse{}
			resp.Result.AccessKey.AccessKeyId = "AKIAIOSFODNN7EXAMPLE"
			resp.Result.AccessKey.SecretAccessKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
			resp.Result.AccessKey.UserName = userName
			data, _ := xml.Marshal(resp)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)

		case "DeleteAccessKey":
			if r.FormValue("AccessKeyId") == "" {
				http.Error(w, "missing AccessKeyId", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<DeleteAccessKeyResponse/>"))

		case "PutUserPolicy":
			if r.FormValue("UserName") == "" || r.FormValue("PolicyName") == "" || r.FormValue("PolicyDocument") == "" {
				http.Error(w, "missing params", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<PutUserPolicyResponse/>"))

		default:
			http.Error(w, "unknown action: "+action, http.StatusBadRequest)
		}
	})

	// Health check
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	client := NewClient(server.URL)
	t.Cleanup(server.Close)
	return server, client
}

func TestCreateUser(t *testing.T) {
	_, client := mockSeaweedFS(t)

	if err := client.CreateUser("test-user"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
}

func TestDeleteUser(t *testing.T) {
	_, client := mockSeaweedFS(t)

	if err := client.DeleteUser("test-user"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
}

func TestCreateAccessKey(t *testing.T) {
	_, client := mockSeaweedFS(t)

	result, err := client.CreateAccessKey("test-user")
	if err != nil {
		t.Fatalf("CreateAccessKey: %v", err)
	}
	if result.AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("expected AccessKeyID AKIAIOSFODNN7EXAMPLE, got %s", result.AccessKeyID)
	}
	if result.SecretAccessKey != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("expected SecretAccessKey wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY, got %s", result.SecretAccessKey)
	}
	if result.UserName != "test-user" {
		t.Errorf("expected UserName test-user, got %s", result.UserName)
	}
}

func TestDeleteAccessKey(t *testing.T) {
	_, client := mockSeaweedFS(t)

	if err := client.DeleteAccessKey("AKIAIOSFODNN7EXAMPLE"); err != nil {
		t.Fatalf("DeleteAccessKey: %v", err)
	}
}

func TestPutUserPolicy(t *testing.T) {
	_, client := mockSeaweedFS(t)

	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"arn:aws:s3:::my-bucket/*"}]}`
	if err := client.PutUserPolicy("test-user", "bucket-access", policy); err != nil {
		t.Fatalf("PutUserPolicy: %v", err)
	}
}

func TestHealth(t *testing.T) {
	_, client := mockSeaweedFS(t)

	if err := client.Health(); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestClientErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if err := client.CreateUser("fail"); err == nil {
		t.Fatal("expected error for 500 response")
	}
	// Health only checks reachability — any HTTP response means the server is up.
	if err := client.Health(); err != nil {
		t.Fatalf("Health should succeed for reachable server: %v", err)
	}
}
