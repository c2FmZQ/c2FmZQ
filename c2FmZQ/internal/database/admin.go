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

// Package database implements all the storage requirement of the c2FmZQ
// server.

package database

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

var (
	ErrOutdated = errors.New("data is out of date")
)

// AdminData encapsulates all the data shown on the admin console.
type AdminData struct {
	Tag              string      `json:"tag"`
	Users            []AdminUser `json:"users,omitempty"`
	DefaultQuota     *int64      `json:"defaultQuota,omitempty"`
	DefaultQuotaUnit *string     `json:"defaultQuotaUnit,omitempty"`
}

// AdminUser encapsulates the user fields that are displayed on the admin
// console.
type AdminUser struct {
	UserID    int64   `json:"userId"`
	Email     *string `json:"email,omitempty"`
	Locked    *bool   `json:"locked,omitempty"`
	Approved  *bool   `json:"approved,omitempty"`
	Admin     *bool   `json:"admin,omitempty"`
	Quota     *int64  `json:"quota,omitempty"`
	QuotaUnit *string `json:"quotaUnit,omitempty"`
}

// AdminData returns the data to display on the admin console.
func (d *Database) AdminData(changes *AdminData) (data *AdminData, retErr error) {
	var quotas Quotas
	files := []string{d.filePath(quotaFile)}
	objects := []interface{}{&quotas}
	users := make(map[int64]*User)

	var ul []userList
	if err := d.storage.ReadDataFile(d.filePath(userListFile), &ul); err != nil {
		return nil, err
	}
	for _, u := range ul {
		if len(u.Email) > 0 && u.Email[0] == '!' {
			continue
		}
		files = append(files, d.filePath(homeByUserID(u.UserID, userFile)))
		var user User
		objects = append(objects, &user)
		users[u.UserID] = &user
	}

	commit, err := d.storage.OpenManyForUpdate(files, objects)
	if err != nil {
		return nil, err
	}
	defer commit(false, &retErr)

	adminData := &AdminData{
		DefaultQuota:     &quotas.DefaultLimit,
		DefaultQuotaUnit: &quotas.DefaultLimitUnit,
	}
	for _, user := range users {
		approved := !user.NeedApproval
		var quota *int64
		var quotaUnit *string
		if v, ok := quotas.Limits[user.UserID]; ok {
			quota = &v.Value
			quotaUnit = &v.Unit
		}
		adminData.Users = append(adminData.Users, AdminUser{
			UserID:    user.UserID,
			Email:     &user.Email,
			Locked:    &user.LoginDisabled,
			Approved:  &approved,
			Admin:     &user.Admin,
			Quota:     quota,
			QuotaUnit: quotaUnit,
		})
	}
	sort.Slice(adminData.Users, func(i, j int) bool {
		return *adminData.Users[i].Email < *adminData.Users[j].Email
	})
	b, err := json.Marshal(adminData)
	if err != nil {
		return nil, err
	}
	h := sha1.Sum(b)
	adminData.Tag = hex.EncodeToString(h[:])
	if changes == nil {
		commit(false, nil)
		return adminData, nil
	}

	if adminData.Tag != changes.Tag {
		return nil, ErrOutdated
	}

	// Apply the changes.
	if changes.DefaultQuota != nil {
		quotas.DefaultLimit = *changes.DefaultQuota
	}
	if changes.DefaultQuotaUnit != nil {
		quotas.DefaultLimitUnit = *changes.DefaultQuotaUnit
	}
	for _, user := range changes.Users {
		if user.Locked != nil {
			users[user.UserID].LoginDisabled = *user.Locked
			if *user.Locked {
				users[user.UserID].ValidTokens = make(map[string]bool)
			}
		}
		if user.Approved != nil {
			users[user.UserID].NeedApproval = !*user.Approved
		}
		if user.Admin != nil {
			users[user.UserID].Admin = *user.Admin
		}
		if user.Quota != nil {
			if *user.Quota < 0 {
				delete(quotas.Limits, user.UserID)
			} else {
				l := quotas.Limits[user.UserID]
				l.Value = *user.Quota
				if u := user.QuotaUnit; u != nil {
					l.Unit = *u
				}
				quotas.Limits[user.UserID] = l
			}
		} else if user.QuotaUnit != nil {
			l := quotas.Limits[user.UserID]
			l.Unit = *user.QuotaUnit
			quotas.Limits[user.UserID] = l
		}
	}

	if err := commit(true, nil); err != nil {
		return nil, err
	}
	return d.AdminData(nil)
}
