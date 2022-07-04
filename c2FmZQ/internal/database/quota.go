//
// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

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
