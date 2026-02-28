package workeradapter

import (
	"testing"

	"github.com/cryguy/worker/v2"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupQueueTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if err := db.AutoMigrate(&QueueMessage{}); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	return db
}

func TestGORMQueueSender_Send(t *testing.T) {
	db := setupQueueTestDB(t)
	siteID := "test-site"
	queueName := "my-queue"

	sender := &GORMQueueSender{
		DB:        db,
		SiteID:    siteID,
		QueueName: queueName,
	}

	// Send a message
	body := `{"key": "value"}`
	contentType := "application/json"
	msgID, err := sender.Send(body, contentType)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if msgID == "" {
		t.Fatal("expected non-empty message ID")
	}

	// Consume the message
	consumer := &GORMQueueConsumer{
		DB:     db,
		SiteID: siteID,
	}
	msgs, err := consumer.Consume(queueName, 10)
	if err != nil {
		t.Fatalf("Consume failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	// Verify message content
	msg := msgs[0]
	if msg.ID != msgID {
		t.Errorf("expected ID %s, got %s", msgID, msg.ID)
	}
	if msg.Body != body {
		t.Errorf("expected body %q, got %q", body, msg.Body)
	}
	if msg.ContentType != contentType {
		t.Errorf("expected content type %q, got %q", contentType, msg.ContentType)
	}
	if msg.SiteID != siteID {
		t.Errorf("expected site ID %s, got %s", siteID, msg.SiteID)
	}
	if msg.Acked {
		t.Error("expected message to not be acked")
	}
}

func TestGORMQueueSender_SendBatch(t *testing.T) {
	db := setupQueueTestDB(t)
	siteID := "test-site"
	queueName := "batch-queue"

	sender := &GORMQueueSender{
		DB:        db,
		SiteID:    siteID,
		QueueName: queueName,
	}

	// Send a batch of messages
	messages := []worker.QueueMessageInput{
		{Body: `{"msg": 1}`, ContentType: "application/json"},
		{Body: `{"msg": 2}`, ContentType: "application/json"},
		{Body: `{"msg": 3}`, ContentType: "application/json"},
	}
	ids, err := sender.SendBatch(messages)
	if err != nil {
		t.Fatalf("SendBatch failed: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}

	// Consume all messages
	consumer := &GORMQueueConsumer{
		DB:     db,
		SiteID: siteID,
	}
	msgs, err := consumer.Consume(queueName, 10)
	if err != nil {
		t.Fatalf("Consume failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Verify all messages were received
	for i, msg := range msgs {
		expectedBody := messages[i].Body
		if msg.Body != expectedBody {
			t.Errorf("message %d: expected body %q, got %q", i, expectedBody, msg.Body)
		}
	}
}

func TestGORMQueueSender_SendDefaultContentType(t *testing.T) {
	db := setupQueueTestDB(t)
	siteID := "test-site"
	queueName := "default-ct-queue"

	sender := &GORMQueueSender{
		DB:        db,
		SiteID:    siteID,
		QueueName: queueName,
	}

	// Send with empty content type
	msgID, err := sender.Send(`{"test": true}`, "")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Consume and verify default content type
	consumer := &GORMQueueConsumer{
		DB:     db,
		SiteID: siteID,
	}
	msgs, err := consumer.Consume(queueName, 10)
	if err != nil {
		t.Fatalf("Consume failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	msg := msgs[0]
	if msg.ID != msgID {
		t.Errorf("expected ID %s, got %s", msgID, msg.ID)
	}
	if msg.ContentType != "json" {
		t.Errorf("expected default content type 'json', got %q", msg.ContentType)
	}
}

func TestGORMQueueConsumer_Ack(t *testing.T) {
	db := setupQueueTestDB(t)
	siteID := "test-site"
	queueName := "ack-queue"

	sender := &GORMQueueSender{
		DB:        db,
		SiteID:    siteID,
		QueueName: queueName,
	}

	// Send a message
	msgID, err := sender.Send(`{"ack": "test"}`, "application/json")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	consumer := &GORMQueueConsumer{
		DB:     db,
		SiteID: siteID,
	}

	// Consume before ack
	msgs, err := consumer.Consume(queueName, 10)
	if err != nil {
		t.Fatalf("Consume failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message before ack, got %d", len(msgs))
	}

	// Ack the message
	if err := consumer.Ack(msgID); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	// Consume after ack should return 0 messages
	msgs, err = consumer.Consume(queueName, 10)
	if err != nil {
		t.Fatalf("Consume after ack failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages after ack, got %d", len(msgs))
	}
}

func TestGORMQueueConsumer_DefaultBatchSize(t *testing.T) {
	db := setupQueueTestDB(t)
	siteID := "test-site"
	queueName := "batch-size-queue"

	sender := &GORMQueueSender{
		DB:        db,
		SiteID:    siteID,
		QueueName: queueName,
	}

	// Send 3 messages
	for i := 0; i < 3; i++ {
		if _, err := sender.Send(`{"index": `+string(rune('0'+i))+`}`, "application/json"); err != nil {
			t.Fatalf("Send message %d failed: %v", i, err)
		}
	}

	consumer := &GORMQueueConsumer{
		DB:     db,
		SiteID: siteID,
	}

	// Consume with batchSize 0 (should default to 10 and return all 3)
	msgs, err := consumer.Consume(queueName, 0)
	if err != nil {
		t.Fatalf("Consume with batchSize 0 failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages with batchSize 0, got %d", len(msgs))
	}

	// Ack all messages
	for _, msg := range msgs {
		if err := consumer.Ack(msg.ID); err != nil {
			t.Fatalf("Ack failed: %v", err)
		}
	}

	// Send 3 more messages
	for i := 0; i < 3; i++ {
		if _, err := sender.Send(`{"index": `+string(rune('3'+i))+`}`, "application/json"); err != nil {
			t.Fatalf("Send message %d failed: %v", i+3, err)
		}
	}

	// Consume with batchSize -1 (should default to 10 and return all 3)
	msgs, err = consumer.Consume(queueName, -1)
	if err != nil {
		t.Fatalf("Consume with batchSize -1 failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages with batchSize -1, got %d", len(msgs))
	}
}

func TestGenerateQueueID_Uniqueness(t *testing.T) {
	const count = 1000
	seen := make(map[string]struct{}, count)

	for i := 0; i < count; i++ {
		id := generateQueueID()
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate ID generated at iteration %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestGenerateQueueID_Format(t *testing.T) {
	const validChars = "abcdefghijklmnopqrstuvwxyz0123456789"
	validSet := make(map[byte]bool, len(validChars))
	for i := 0; i < len(validChars); i++ {
		validSet[validChars[i]] = true
	}

	for i := 0; i < 100; i++ {
		id := generateQueueID()

		// Must start with "qm_" prefix.
		if len(id) < 4 || id[:3] != "qm_" {
			t.Fatalf("ID %q does not start with 'qm_'", id)
		}

		// Suffix must be 12 lowercase alphanumeric characters.
		suffix := id[3:]
		if len(suffix) != 12 {
			t.Errorf("ID suffix length = %d, want 12 (full ID: %q)", len(suffix), id)
			continue
		}
		for j := 0; j < len(suffix); j++ {
			if !validSet[suffix[j]] {
				t.Errorf("ID %q contains invalid character %q at position %d", id, suffix[j], j+3)
			}
		}
	}
}

func TestGORMQueueSender_SiteIsolation(t *testing.T) {
	db := setupQueueTestDB(t)

	senderA := &GORMQueueSender{DB: db, SiteID: "siteA", QueueName: "shared-queue"}
	senderB := &GORMQueueSender{DB: db, SiteID: "siteB", QueueName: "shared-queue"}

	senderA.Send(`{"from":"A"}`, "json")
	senderB.Send(`{"from":"B"}`, "json")

	consumerA := &GORMQueueConsumer{DB: db, SiteID: "siteA"}
	consumerB := &GORMQueueConsumer{DB: db, SiteID: "siteB"}

	msgsA, err := consumerA.Consume("shared-queue", 10)
	if err != nil {
		t.Fatalf("Consume siteA: %v", err)
	}
	if len(msgsA) != 1 {
		t.Errorf("siteA should see 1 message, got %d", len(msgsA))
	}
	if len(msgsA) > 0 && msgsA[0].Body != `{"from":"A"}` {
		t.Errorf("siteA message body = %q", msgsA[0].Body)
	}

	msgsB, err := consumerB.Consume("shared-queue", 10)
	if err != nil {
		t.Fatalf("Consume siteB: %v", err)
	}
	if len(msgsB) != 1 {
		t.Errorf("siteB should see 1 message, got %d", len(msgsB))
	}
	if len(msgsB) > 0 && msgsB[0].Body != `{"from":"B"}` {
		t.Errorf("siteB message body = %q", msgsB[0].Body)
	}
}
