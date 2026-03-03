package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/models"
)

// seedAuditLog inserts an AuditLog directly into the test DB.
func seedAuditLog(t *testing.T, env *testEnv, actorID, actorEmail, action, resourceType, resourceID string, createdAt time.Time) models.AuditLog {
	t.Helper()
	entry := models.AuditLog{
		ActorID:      actorID,
		ActorEmail:   actorEmail,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		CreatedAt:    createdAt,
	}
	if err := env.db.Create(&entry).Error; err != nil {
		t.Fatalf("seedAuditLog: %v", err)
	}
	return entry
}

// auditListResponse mirrors the handler's JSON shape for decoding.
type auditListResponse struct {
	Items []models.AuditLog `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
}

func decodeAuditResponse(t *testing.T, rec *httptest.ResponseRecorder) auditListResponse {
	t.Helper()
	var resp auditListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	return resp
}

// ──────────────────────────────────────────────
// Auth
// ──────────────────────────────────────────────

func TestAuditLogs_RequiresAuth(t *testing.T) {
	env := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	rec := env.doRequest(req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// ──────────────────────────────────────────────
// Authorization scoping
// ──────────────────────────────────────────────

func TestAuditLogs_RegularUserSeesOnlyOwnLogs(t *testing.T) {
	env := setupTestEnv(t)

	userA, tokenA := env.createTestUser(t, "a@test.com", "password123", "user")
	userB, _ := env.createTestUser(t, "b@test.com", "password123", "user")

	seedAuditLog(t, env, userA.ID, userA.Email, "site.create", "site", "s1", time.Now())
	seedAuditLog(t, env, userA.ID, userA.Email, "site.delete", "site", "s2", time.Now())
	seedAuditLog(t, env, userB.ID, userB.Email, "site.create", "site", "s3", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2 (only user A's logs)", resp.Total)
	}
	for _, item := range resp.Items {
		if item.ActorID != userA.ID {
			t.Errorf("got log for actor %q, want only %q", item.ActorID, userA.ID)
		}
	}
}

func TestAuditLogs_AdminSeesAllLogs(t *testing.T) {
	env := setupTestEnv(t)

	userA, _ := env.createTestUser(t, "a@test.com", "password123", "user")
	userB, _ := env.createTestUser(t, "b@test.com", "password123", "user")
	_, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	seedAuditLog(t, env, userA.ID, userA.Email, "site.create", "site", "s1", time.Now())
	seedAuditLog(t, env, userB.ID, userB.Email, "site.create", "site", "s2", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2 (admin sees all logs)", resp.Total)
	}
}

func TestAuditLogs_SuperadminSeesAllLogs(t *testing.T) {
	env := setupTestEnv(t)

	userA, _ := env.createTestUser(t, "a@test.com", "password123", "user")
	_, superToken := env.createTestUser(t, "super@test.com", "password123", "superadmin")

	seedAuditLog(t, env, userA.ID, userA.Email, "site.create", "site", "s1", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer "+superToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
}

// ──────────────────────────────────────────────
// Pagination
// ──────────────────────────────────────────────

func TestAuditLogs_Pagination_PageOneFiftyItems(t *testing.T) {
	env := setupTestEnv(t)

	admin, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	// Seed 60 entries so page 1 = 50, page 2 = 10.
	for i := 0; i < 60; i++ {
		seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site",
			fmt.Sprintf("s%02d", i), time.Now().Add(-time.Duration(i)*time.Second))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?page=1", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 60 {
		t.Errorf("total = %d, want 60", resp.Total)
	}
	if len(resp.Items) != 50 {
		t.Errorf("items on page 1 = %d, want 50", len(resp.Items))
	}
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Page)
	}
}

func TestAuditLogs_Pagination_PageTwoHasRemainder(t *testing.T) {
	env := setupTestEnv(t)

	admin, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	for i := 0; i < 60; i++ {
		seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site",
			fmt.Sprintf("s%02d", i), time.Now().Add(-time.Duration(i)*time.Second))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?page=2", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if len(resp.Items) != 10 {
		t.Errorf("items on page 2 = %d, want 10", len(resp.Items))
	}
	if resp.Page != 2 {
		t.Errorf("page = %d, want 2", resp.Page)
	}
}

func TestAuditLogs_InvalidPageDefaultsToOne_ZeroValue(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?page=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1 (page=0 should default to 1)", resp.Page)
	}
}

func TestAuditLogs_InvalidPageDefaultsToOne_NegativeValue(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?page=-5", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1 (page=-5 should default to 1)", resp.Page)
	}
}

func TestAuditLogs_InvalidPageDefaultsToOne_NonNumeric(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?page=abc", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1 (non-numeric page should default to 1)", resp.Page)
	}
}

// ──────────────────────────────────────────────
// Filters
// ──────────────────────────────────────────────

func TestAuditLogs_FilterByAction(t *testing.T) {
	env := setupTestEnv(t)
	admin, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "s1", time.Now())
	seedAuditLog(t, env, admin.ID, admin.Email, "site.delete", "site", "s2", time.Now())
	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "s3", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?action=site.create", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2 (only site.create actions)", resp.Total)
	}
	for _, item := range resp.Items {
		if item.Action != "site.create" {
			t.Errorf("got action %q, want site.create", item.Action)
		}
	}
}

func TestAuditLogs_FilterByResourceType(t *testing.T) {
	env := setupTestEnv(t)
	admin, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "s1", time.Now())
	seedAuditLog(t, env, admin.ID, admin.Email, "deploy.create", "deployment", "d1", time.Now())
	seedAuditLog(t, env, admin.ID, admin.Email, "site.delete", "site", "s2", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?resource_type=site", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2 (only site resource type)", resp.Total)
	}
	for _, item := range resp.Items {
		if item.ResourceType != "site" {
			t.Errorf("got resource_type %q, want site", item.ResourceType)
		}
	}
}

func TestAuditLogs_FilterByActorID(t *testing.T) {
	env := setupTestEnv(t)

	userA, _ := env.createTestUser(t, "a@test.com", "password123", "user")
	userB, _ := env.createTestUser(t, "b@test.com", "password123", "user")
	_, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	seedAuditLog(t, env, userA.ID, userA.Email, "site.create", "site", "s1", time.Now())
	seedAuditLog(t, env, userB.ID, userB.Email, "site.create", "site", "s2", time.Now())
	seedAuditLog(t, env, userA.ID, userA.Email, "site.delete", "site", "s3", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?actor_id="+userA.ID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2 (only actor A's logs)", resp.Total)
	}
	for _, item := range resp.Items {
		if item.ActorID != userA.ID {
			t.Errorf("got actor_id %q, want %q", item.ActorID, userA.ID)
		}
	}
}

func TestAuditLogs_FilterByResourceID(t *testing.T) {
	env := setupTestEnv(t)
	admin, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "target-site", time.Now())
	seedAuditLog(t, env, admin.ID, admin.Email, "site.delete", "site", "other-site", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?resource_id=target-site", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1 (only target-site)", resp.Total)
	}
	if len(resp.Items) > 0 && resp.Items[0].ResourceID != "target-site" {
		t.Errorf("resource_id = %q, want target-site", resp.Items[0].ResourceID)
	}
}

func TestAuditLogs_FilterByDateRange_FromExcludesBefore(t *testing.T) {
	env := setupTestEnv(t)
	admin, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	boundary := time.Now().UTC().Add(-24 * time.Hour)

	// Entry from 48h ago — should be excluded.
	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "s1",
		boundary.Add(-24*time.Hour))

	// Entry from 12h ago — should be included.
	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "s2",
		boundary.Add(12*time.Hour))

	from := boundary.Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?from="+from, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1 (only entry after 'from')", resp.Total)
	}
	if len(resp.Items) > 0 && resp.Items[0].ResourceID != "s2" {
		t.Errorf("resource_id = %q, want s2", resp.Items[0].ResourceID)
	}
}

func TestAuditLogs_FilterByDateRange_ToExcludesAfter(t *testing.T) {
	env := setupTestEnv(t)
	admin, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	boundary := time.Now().UTC().Add(-12 * time.Hour)

	// Entry from 24h ago — should be included (before boundary).
	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "s1",
		boundary.Add(-12*time.Hour))

	// Entry from 1h ago — should be excluded (after boundary).
	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "s2",
		boundary.Add(6*time.Hour))

	to := boundary.Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?to="+to, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1 (only entry before 'to')", resp.Total)
	}
	if len(resp.Items) > 0 && resp.Items[0].ResourceID != "s1" {
		t.Errorf("resource_id = %q, want s1", resp.Items[0].ResourceID)
	}
}

func TestAuditLogs_FilterByDateRange_InvalidFromIsIgnored(t *testing.T) {
	env := setupTestEnv(t)
	admin, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "s1", time.Now())

	// Invalid from date — should be silently ignored, all entries returned.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?from=not-a-date", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1 (invalid from is ignored)", resp.Total)
	}
}

func TestAuditLogs_FilterByDateRange_InvalidToIsIgnored(t *testing.T) {
	env := setupTestEnv(t)
	admin, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "s1", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?to=not-a-date", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1 (invalid to is ignored)", resp.Total)
	}
}

// ──────────────────────────────────────────────
// Response shape
// ──────────────────────────────────────────────

func TestAuditLogs_EmptyResultsReturnValidShape(t *testing.T) {
	env := setupTestEnv(t)
	_, token := env.createTestUser(t, "user@test.com", "password123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if resp.Items == nil {
		t.Error("items should not be nil (expect empty array, not null)")
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Page)
	}
}

func TestAuditLogs_ResponseIsOrderedByCreatedAtDesc(t *testing.T) {
	env := setupTestEnv(t)
	admin, adminToken := env.createTestUser(t, "admin@test.com", "password123", "admin")

	base := time.Now().UTC().Add(-3 * time.Hour)
	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "oldest", base)
	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "middle", base.Add(time.Hour))
	seedAuditLog(t, env, admin.ID, admin.Email, "site.create", "site", "newest", base.Add(2*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := env.doRequest(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAuditResponse(t, rec)
	if len(resp.Items) != 3 {
		t.Fatalf("got %d items, want 3", len(resp.Items))
	}

	// First item should be the newest.
	if resp.Items[0].ResourceID != "newest" {
		t.Errorf("first item resource_id = %q, want newest (descending order)", resp.Items[0].ResourceID)
	}
	if resp.Items[2].ResourceID != "oldest" {
		t.Errorf("last item resource_id = %q, want oldest (descending order)", resp.Items[2].ResourceID)
	}
}
