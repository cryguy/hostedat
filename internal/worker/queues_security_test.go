package worker

import (
	"testing"
)

// TestQueue_SiteIsolation verifies that queue names are isolated by site ID.
func TestQueue_SiteIsolation(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})

	bridge1 := &QueueBridge{DB: db, SiteID: "site-a"}
	bridge2 := &QueueBridge{DB: db, SiteID: "site-b"}

	// Both send to "myqueue"
	_, err := bridge1.Send("myqueue", `{"msg":"from-a"}`, "json")
	if err != nil {
		t.Fatalf("bridge1.Send: %v", err)
	}
	_, err = bridge2.Send("myqueue", `{"msg":"from-b"}`, "json")
	if err != nil {
		t.Fatalf("bridge2.Send: %v", err)
	}

	// Verify messages are isolated by checking the actual queue name in DB
	var messages []QueueMessage
	if err := db.Find(&messages).Error; err != nil {
		t.Fatalf("query: %v", err)
	}

	siteACount := 0
	siteBCount := 0
	for _, m := range messages {
		if m.QueueName == "site-a:myqueue" {
			siteACount++
		}
		if m.QueueName == "site-b:myqueue" {
			siteBCount++
		}
	}
	if siteACount != 1 {
		t.Errorf("site-a messages = %d, want 1", siteACount)
	}
	if siteBCount != 1 {
		t.Errorf("site-b messages = %d, want 1", siteBCount)
	}
}
