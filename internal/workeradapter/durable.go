package workeradapter

import (
	"errors"

	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/worker"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Compile-time interface check.
var _ worker.DurableObjectStore = (*GORMDurableObjectStore)(nil)

// GORMDurableObjectStore implements worker.DurableObjectStore using GORM.
type GORMDurableObjectStore struct {
	DB *gorm.DB
}

// Get retrieves a single value by key, returning "" if not found.
func (b *GORMDurableObjectStore) Get(namespace, objectID, key string) (string, error) {
	var entry models.DurableObjectEntry
	err := b.DB.Where("namespace = ? AND object_id = ? AND key = ?", namespace, objectID, key).First(&entry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return entry.ValueJSON, nil
}

// GetMulti retrieves multiple values by keys.
func (b *GORMDurableObjectStore) GetMulti(namespace, objectID string, keys []string) (map[string]string, error) {
	var entries []models.DurableObjectEntry
	err := b.DB.Where("namespace = ? AND object_id = ? AND key IN ?", namespace, objectID, keys).Find(&entries).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(entries))
	for _, e := range entries {
		result[e.Key] = e.ValueJSON
	}
	return result, nil
}

// Put upserts a single key-value pair.
func (b *GORMDurableObjectStore) Put(namespace, objectID, key, valueJSON string) error {
	entry := models.DurableObjectEntry{
		Namespace: namespace,
		ObjectID:  objectID,
		Key:       key,
		ValueJSON: valueJSON,
	}
	return b.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "namespace"}, {Name: "object_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value_json"}),
	}).Create(&entry).Error
}

// PutMulti upserts multiple key-value pairs.
func (b *GORMDurableObjectStore) PutMulti(namespace, objectID string, entries map[string]string) error {
	return b.DB.Transaction(func(tx *gorm.DB) error {
		for k, v := range entries {
			entry := models.DurableObjectEntry{
				Namespace: namespace,
				ObjectID:  objectID,
				Key:       k,
				ValueJSON: v,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "namespace"}, {Name: "object_id"}, {Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value_json"}),
			}).Create(&entry).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Delete removes a single key.
func (b *GORMDurableObjectStore) Delete(namespace, objectID, key string) error {
	return b.DB.Where("namespace = ? AND object_id = ? AND key = ?", namespace, objectID, key).
		Delete(&models.DurableObjectEntry{}).Error
}

// DeleteMulti removes multiple keys, returning the count deleted.
func (b *GORMDurableObjectStore) DeleteMulti(namespace, objectID string, keys []string) (int, error) {
	result := b.DB.Where("namespace = ? AND object_id = ? AND key IN ?", namespace, objectID, keys).
		Delete(&models.DurableObjectEntry{})
	return int(result.RowsAffected), result.Error
}

// DeleteAll removes all entries for a given object.
func (b *GORMDurableObjectStore) DeleteAll(namespace, objectID string) error {
	return b.DB.Where("namespace = ? AND object_id = ?", namespace, objectID).
		Delete(&models.DurableObjectEntry{}).Error
}

// List returns entries matching an optional prefix, with limit and reverse support.
func (b *GORMDurableObjectStore) List(namespace, objectID, prefix string, limit int, reverse bool) ([]worker.KVPair, error) {
	if limit <= 0 {
		limit = 128
	}

	q := b.DB.Where("namespace = ? AND object_id = ?", namespace, objectID)
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}
	if reverse {
		q = q.Order("key DESC")
	} else {
		q = q.Order("key ASC")
	}

	var entries []models.DurableObjectEntry
	if err := q.Limit(limit).Find(&entries).Error; err != nil {
		return nil, err
	}

	result := make([]worker.KVPair, len(entries))
	for i, e := range entries {
		result[i] = worker.KVPair{Key: e.Key, Value: e.ValueJSON}
	}
	return result, nil
}
