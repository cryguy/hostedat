package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// ──────────────────────────────────────────────
// errorJSON tests
// ──────────────────────────────────────────────

func TestErrorJSON_ReturnsCorrectFormat(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := errorJSON(c, http.StatusBadRequest, "test error message")
	if err != nil {
		t.Fatalf("errorJSON returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}

	expected := `{"error":"test error message"}`
	got := rec.Body.String()
	if got != expected+"\n" {
		t.Errorf("body = %q, want %q", got, expected)
	}
}

func TestErrorJSON_DifferentStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		message    string
	}{
		{"BadRequest", http.StatusBadRequest, "bad request"},
		{"Unauthorized", http.StatusUnauthorized, "unauthorized"},
		{"Forbidden", http.StatusForbidden, "forbidden"},
		{"NotFound", http.StatusNotFound, "not found"},
		{"Conflict", http.StatusConflict, "conflict"},
		{"InternalServerError", http.StatusInternalServerError, "internal error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			errorJSON(c, tt.statusCode, tt.message)

			if rec.Code != tt.statusCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.statusCode)
			}
		})
	}
}

// ──────────────────────────────────────────────
// CustomErrorHandler tests
// ──────────────────────────────────────────────

func TestCustomErrorHandler_EchoHTTPError(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = CustomErrorHandler

	e.GET("/test", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusNotFound, "resource not found")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}

	expected := `{"error":"resource not found"}`
	got := rec.Body.String()
	if got != expected+"\n" {
		t.Errorf("body = %q, want %q", got, expected)
	}
}

func TestCustomErrorHandler_EchoHTTPErrorWithoutMessage(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = CustomErrorHandler

	e.GET("/test", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusBadRequest)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}

	expected := `{"error":"Bad Request"}`
	got := rec.Body.String()
	if got != expected+"\n" {
		t.Errorf("body = %q, want %q", got, expected)
	}
}

func TestCustomErrorHandler_PlainError(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = CustomErrorHandler

	e.GET("/test", func(c echo.Context) error {
		return errors.New("generic error")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}

	expected := `{"error":"internal server error"}`
	got := rec.Body.String()
	if got != expected+"\n" {
		t.Errorf("body = %q, want %q", got, expected)
	}
}

func TestCustomErrorHandler_ResponseAlreadyCommitted(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = CustomErrorHandler

	e.GET("/test", func(c echo.Context) error {
		// Commit the response first
		c.Response().WriteHeader(http.StatusOK)
		c.Response().Write([]byte("already sent"))
		// Then return an error (should be ignored)
		return errors.New("too late")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (response already committed)", rec.Code)
	}

	got := rec.Body.String()
	if got != "already sent" {
		t.Errorf("body = %q, want 'already sent'", got)
	}
}

func TestCustomErrorHandler_NonStringMessage(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = CustomErrorHandler

	e.GET("/test", func(c echo.Context) error {
		// HTTPError with non-string message
		err := echo.NewHTTPError(http.StatusBadRequest)
		err.Message = 12345 // non-string
		return err
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}

	// Should fall back to http.StatusText
	expected := `{"error":"Bad Request"}`
	got := rec.Body.String()
	if got != expected+"\n" {
		t.Errorf("body = %q, want %q", got, expected)
	}
}

func TestErrorResponse_StructFormat(t *testing.T) {
	resp := ErrorResponse{Error: "test message"}
	if resp.Error != "test message" {
		t.Errorf("Error = %q, want 'test message'", resp.Error)
	}
}
