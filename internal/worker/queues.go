package worker

import (
	"encoding/json"
	"fmt"
	"time"

	v8 "github.com/tommie/v8go"
	"gorm.io/gorm"
)

// QueueMessage represents a message in a queue, stored in SQLite via GORM.
type QueueMessage struct {
	ID          string `gorm:"primaryKey"`
	QueueName   string `gorm:"index"`
	Body        string
	ContentType string
	CreatedAt   time.Time
	Acked       bool `gorm:"default:false"`
}

// BeforeCreate generates a nanoid for QueueMessage if not set.
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

// QueueMessageInput is the Go representation of a batch send item.
type QueueMessageInput struct {
	Body        string
	ContentType string
}

// QueueBridge provides Go methods that back the Queue JS bindings.
type QueueBridge struct {
	DB     *gorm.DB
	SiteID string
}

// Send creates a single message in the given queue and returns its ID.
func (q *QueueBridge) Send(queueName, body, contentType string) (string, error) {
	if contentType == "" {
		contentType = "json"
	}
	// Prefix queue name with siteID for isolation between sites.
	actualName := q.SiteID + ":" + queueName
	msg := QueueMessage{
		QueueName:   actualName,
		Body:        body,
		ContentType: contentType,
		CreatedAt:   time.Now(),
	}
	if err := q.DB.Create(&msg).Error; err != nil {
		return "", err
	}
	return msg.ID, nil
}

// SendBatch creates multiple messages in the given queue and returns their IDs.
func (q *QueueBridge) SendBatch(queueName string, messages []QueueMessageInput) ([]string, error) {
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		id, err := q.Send(queueName, m.Body, m.ContentType)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// Ack marks a message as acknowledged.
func (q *QueueBridge) Ack(messageID string) error {
	return q.DB.Model(&QueueMessage{}).Where("id = ?", messageID).Update("acked", true).Error
}

// Consume retrieves up to batchSize unacked messages from the queue.
func (q *QueueBridge) Consume(queueName string, batchSize int) ([]QueueMessage, error) {
	if batchSize <= 0 {
		batchSize = 10
	}
	// Prefix queue name with siteID for isolation between sites.
	actualName := q.SiteID + ":" + queueName
	var msgs []QueueMessage
	err := q.DB.Where("queue_name = ? AND acked = ?", actualName, false).
		Order("created_at ASC").
		Limit(batchSize).
		Find(&msgs).Error
	return msgs, err
}

// buildQueueBinding creates a JS object with async send/sendBatch methods
// backed by the given QueueBridge for a specific queue name.
func buildQueueBinding(iso *v8.Isolate, ctx *v8.Context, bridge *QueueBridge, queueName string) (*v8.Value, error) {
	qObj, err := newJSObject(iso, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating Queue object: %w", err)
	}

	// send(body, options?) -> Promise<void>
	_ = qObj.Set("send", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "Queue.send requires a body argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		body := args[0].String()
		contentType := "json"

		if len(args) > 1 && args[1].IsObject() {
			_ = ctx.Global().Set("__tmp_queue_opts", args[1])
			optsResult, err := ctx.RunScript(`(function() {
				var o = globalThis.__tmp_queue_opts;
				delete globalThis.__tmp_queue_opts;
				return o.contentType !== undefined && o.contentType !== null ? String(o.contentType) : "json";
			})()`, "queue_opts.js")
			if err == nil {
				contentType = optsResult.String()
			}
		}

		if _, err := bridge.Send(queueName, body, contentType); err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(v8.Undefined(iso))
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	// sendBatch(messages) -> Promise<void>
	_ = qObj.Set("sendBatch", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "Queue.sendBatch requires a messages argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		// Extract messages array via JS JSON serialization.
		_ = ctx.Global().Set("__tmp_queue_batch", args[0])
		batchResult, err := ctx.RunScript(`(function() {
			var msgs = globalThis.__tmp_queue_batch;
			delete globalThis.__tmp_queue_batch;
			if (!Array.isArray(msgs)) return JSON.stringify([]);
			return JSON.stringify(msgs.map(function(m) {
				return {
					body: typeof m.body === 'string' ? m.body : JSON.stringify(m.body),
					contentType: m.contentType || "json"
				};
			}));
		})()`, "queue_batch.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, "failed to parse batch messages: "+err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		var inputs []QueueMessageInput
		if err := json.Unmarshal([]byte(batchResult.String()), &inputs); err != nil {
			errVal, _ := v8.NewValue(iso, "failed to unmarshal batch messages: "+err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		if _, err := bridge.SendBatch(queueName, inputs); err != nil {
			errVal, _ := v8.NewValue(iso, err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		resolver.Resolve(v8.Undefined(iso))
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	return qObj.Value, nil
}
