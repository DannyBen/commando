package settings

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

// ValidIdentity also permits short user-chosen names. Generated IDs are only
// a convention; names remain opaque and case-sensitive.
func ValidIdentity(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// NormalizeIdentities assigns missing IDs without a validation error. Invalid
// or duplicate IDs are repaired and reported to the ordinary recovery flow.
func (c Catalogue) NormalizeIdentities(values *Values) error {
	var failures []error
	for collection, field := range c.CollectionIdentities {
		rows := values.Collection(collection)
		if err := normalizeCollectionIdentities(field, rows); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", collection, err))
		}
		if len(rows) > 0 {
			values.SetCollection(collection, rows)
		}
	}
	return errors.Join(failures...)
}

func normalizeCollectionIdentities(field string, rows []map[string]any) error {
	if field == "" {
		return nil
	}
	reserved, seen := map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		if id, ok := row[field].(string); ok && ValidIdentity(id) {
			reserved[id] = true
		}
	}
	var failures []error
	for _, row := range rows {
		raw, exists := row[field]
		id, text := raw.(string)
		if ValidIdentity(id) && !seen[id] {
			seen[id] = true
			continue
		}
		if exists && (!text || id != "") {
			failures = append(failures, fmt.Errorf("%w: invalid or duplicate identity", ErrValue))
		}
		for {
			var bytes [6]byte
			if _, err := rand.Read(bytes[:]); err != nil {
				return errors.Join(append(failures, err)...)
			}
			id = hex.EncodeToString(bytes[:])
			if !reserved[id] {
				break
			}
		}
		reserved[id], seen[id], row[field] = true, true, id
	}
	return errors.Join(failures...)
}
