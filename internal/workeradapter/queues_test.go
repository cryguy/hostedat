package workeradapter

import (
	"testing"

	"github.com/cryguy/worker"
	"gorm.io/driver/sqlite"
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
