package workeradapter

import (
	"fmt"
	"time"

	"github.com/cryguy/worker"
	"gorm.io/gorm"
)

// Compile-time interface check.
var _ worker.QueueSender = (*GORMQueueSender)(nil)

// QueueMessage represents a message in a queue, stored in SQLite via GORM.
type QueueMessage struct {
	ID          string `gorm:"primaryKey"`
	QueueName   string `gorm:"index"`
	Body        string
	ContentType string
	CreatedAt   time.Time
	Acked       bool   `gorm:"default:false"`
	SiteID      string `gorm:"index"`
}

// BeforeCreate generates an ID for QueueMessage if not set.
func (q *QueueMessage) BeforeCreate(_ *gorm.DB) error {
	if q.ID == "" {
		q.ID = generateQueueID()
	}
	return nil
}

// generateQueueID creates a simple unique ID for queue messages.
func generateQueueID() string {
	return fmt.Sprintf("qm_%d", time.Now().UnixNano())
}

// GORMQueueSender implements worker.QueueSender using GORM.
type GORMQueueSender struct {
	DB        *gorm.DB
	SiteID    string
	QueueName string
}

// Send creates a single message in the queue and returns its ID.
func (qs *GORMQueueSender) Send(body, contentType string) (string, error) {
	if contentType == "" {
		contentType = "json"
	}
	// Prefix queue name with siteID for isolation between sites.
	actualName := qs.SiteID + ":" + qs.QueueName
	msg := QueueMessage{
		QueueName:   actualName,
		Body:        body,
		ContentType: contentType,
		CreatedAt:   time.Now(),
		SiteID:      qs.SiteID,
	}
	if err := qs.DB.Create(&msg).Error; err != nil {
		return "", err
	}
	return msg.ID, nil
}

// SendBatch creates multiple messages in the queue and returns their IDs.
func (qs *GORMQueueSender) SendBatch(messages []worker.QueueMessageInput) ([]string, error) {
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		id, err := qs.Send(m.Body, m.ContentType)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// GORMQueueConsumer provides server-side queue message consumption.
type GORMQueueConsumer struct {
	DB     *gorm.DB
	SiteID string
}

// Ack marks a message as acknowledged.
func (qc *GORMQueueConsumer) Ack(messageID string) error {
	return qc.DB.Model(&QueueMessage{}).Where("id = ?", messageID).Update("acked", true).Error
}

// Consume retrieves up to batchSize unacked messages from the queue.
func (qc *GORMQueueConsumer) Consume(queueName string, batchSize int) ([]QueueMessage, error) {
	if batchSize <= 0 {
		batchSize = 10
	}
	// Prefix queue name with siteID for isolation between sites.
	actualName := qc.SiteID + ":" + queueName
	var msgs []QueueMessage
	err := qc.DB.Where("queue_name = ? AND acked = ?", actualName, false).
		Order("created_at ASC").
		Limit(batchSize).
		Find(&msgs).Error
	return msgs, err
}
