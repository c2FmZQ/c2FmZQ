package database

import (
	"strings"

	"c2FmZQ/internal/log"
)

const (
	quotaFile = "quotas.dat"
)

// Quotas contains the quota limits, keyed by user ID.
type Quotas struct {
	Limits       map[int64]Limit `json:"limits"`
	DefaultLimit int64           `json:"defaultLimit"`
}

type Limit struct {
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
}

// Quota returns the user's quota.
func (d *Database) Quota(userID int64) (int64, error) {
	var quotas Quotas
	if err := d.storage.ReadDataFile(d.filePath(quotaFile), &quotas); err != nil {
		return 0, err
	}
	if q, ok := quotas.Limits[userID]; ok {
		limit := q.Value
		switch strings.ToLower(q.Unit) {
		case "k", "kb":
			limit <<= 10
		case "m", "mb":
			limit <<= 20
		case "g", "gb":
			limit <<= 30
		case "t", "tb":
			limit <<= 40
		default:
		}
		return limit, nil
	}
	return quotas.DefaultLimit, nil
}

// CreateEmptyQuotaFile creates an empty quota file with a large default limit.
func (d *Database) CreateEmptyQuotaFile() error {
	q := Quotas{
		Limits:       map[int64]Limit{0: Limit{0, "MB"}}, // Example.
		DefaultLimit: 100 << 40,                          // 100 TB (arbitrarily large value)
	}
	return d.storage.CreateEmptyFile(d.filePath(quotaFile), &q)
}

func (d *Database) EditQuotas() error {
	var quotas Quotas
	if err := d.storage.EditDataFile(d.filePath(quotaFile), &quotas); err != nil {
		log.Errorf("EditDataFile(%q): %v", d.filePath(quotaFile), err)
		return err
	}
	return nil
}
