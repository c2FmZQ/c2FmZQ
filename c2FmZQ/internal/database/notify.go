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
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/webpush"
)

const (
	// The logical filename where the push service configuration is stored.
	pushServiceConfigFile = "push-services.dat"

	// A new user was registered.
	notifyNewUserRegistration = 1
	// New content was added to a shared album.
	notifyNewContent = 2
	// A new member was added to a shared album.
	notifyNewMember = 3
	// A test for the endpoint.
	notifyTest = 4
)

// notification encapsulates the content to be sent with a push notification.
type notification struct {
	ID     int64       `json:"id"`
	Type   int         `json:"type"`
	Target string      `json:"target,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

// notifyItem is a queued notification.
type notifyItem struct {
	uid int64
	n   *notification
}

func makeID() (int64, error) {
	bi, err := rand.Int(rand.Reader, big.NewInt(int64(900000000)))
	if err != nil {
		return 0, err
	}
	return bi.Int64() + 100000000, nil
}

// startNotifyWorkers starts goroutines to process the queue of push
// notifications.
func (db *Database) startNotifyWorkers() {
	if !db.pushServices.Enable {
		return
	}
	worker := func() {
		for q := range db.notifyChan {
			if q.n.ID == 0 {
				id, err := makeID()
				if err != nil {
					log.Errorf("makeID(): %v", err)
					continue
				}
				q.n.ID = id
			}
			u, err := db.UserByID(q.uid)
			if err != nil {
				log.Errorf("db.UserByID(%d): %v", q.uid, err)
				continue
			}
			if err := db.sendNotification(u, q.n); err != nil {
				log.Errorf("sendNotification: %v", err)
				continue
			}
		}
	}
	for i := 0; i < 5; i++ {
		go worker()
	}
}

// enqueueNotification adds a notification to the send. This is done
// asynchronously. If the queue is full, the notification is dropped.
func (db *Database) enqueueNotification(n notifyItem) {
	select {
	case db.notifyChan <- n:
	default:
		log.Error("enqueueNotification: queue is full")
	}
}

// TestPushEndpoint sends a test notifcation to the endpoint, typically after
// the user enabled notifications.
func (db *Database) TestPushEndpoint(user User, ep string) error {
	if !db.pushServices.Enable {
		return errors.New("push notifications disabled")
	}
	pc := user.PushConfig
	if pc == nil || pc.Endpoints[ep] == nil {
		return errors.New("invalid endpoint")
	}
	id, err := makeID()
	if err != nil {
		return err
	}
	b, err := json.Marshal(notification{ID: id, Type: notifyTest})
	if err != nil {
		log.Errorf("Marshal: %v", err)
		return err
	}
	payload := []byte(user.PublicKey.SealBoxBase64(b))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := db.pushServices.Send(ctx, webpush.Params{
		Endpoint:                    ep,
		ApplicationServerPrivateKey: pc.ApplicationServerPrivateKey,
		ApplicationServerPublicKey:  pc.ApplicationServerPublicKey,
		Auth:                        pc.Endpoints[ep].Auth,
		P256dh:                      pc.Endpoints[ep].P256dh,
		Payload:                     payload,
	})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode; sc < 200 || sc >= 300 {
		return errors.New(resp.Status)
	}
	return nil
}

// notifyAdmins sends a notification to all admin users.
func (db *Database) notifyAdmins(n notification) {
	if db.notifyChan == nil || !db.pushServices.Enable {
		return
	}
	var ul []userList
	if err := db.storage.ReadDataFile(db.filePath(userListFile), &ul); err != nil {
		log.Errorf("notifyAdmins: %v", err)
		return
	}
	for _, u := range ul {
		if u.Admin {
			db.enqueueNotification(notifyItem{uid: u.UserID, n: &n})
		}
	}
}

// notifyAlbum sends a notification to the members of a shared album.
func (db *Database) notifyAlbum(originator int64, album *AlbumSpec, n notification) {
	if db.notifyChan == nil || !db.pushServices.Enable || album == nil || !album.IsShared {
		return
	}
	uids := map[int64]bool{
		album.OwnerID: true,
	}
	for id := range album.Members {
		uids[id] = true
	}
	//delete(uids, originator)

	for id := range uids {
		db.enqueueNotification(notifyItem{uid: id, n: &n})
	}
}

// NewPushConfig returns a new PushConfig with a fresh key.
func NewPushConfig() (*PushConfig, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Errorf("ecdsa.GenerateKey: %v", err)
		return nil, err
	}
	b, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		log.Errorf("x509.MarshalECPrivateKey: %v", err)
		return nil, err
	}
	return &PushConfig{
		ApplicationServerPrivateKey: base64.RawURLEncoding.EncodeToString(b),
		ApplicationServerPublicKey:  base64.RawURLEncoding.EncodeToString(elliptic.Marshal(key.PublicKey.Curve, key.PublicKey.X, key.PublicKey.Y)),
	}, nil
}

// sendNotification sends a push notification to user's subscribed devices.
func (db *Database) sendNotification(user User, msg interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if !db.pushServices.Enable {
		return errors.New("push notifications disabled")
	}
	pc := user.PushConfig
	if pc == nil || len(pc.Endpoints) == 0 {
		// Nothing to do.
		return nil
	}
	b, err := json.Marshal(msg)
	if err != nil {
		log.Errorf("Marshal: %v", err)
		return err
	}
	payload := []byte(user.PublicKey.SealBoxBase64(b))
	var changed bool
	for ep := range pc.Endpoints {
		if r := pc.Endpoints[ep].RetryAfter; r > time.Now().Unix() {
			continue
		}
		resp, err := db.pushServices.Send(ctx, webpush.Params{
			Endpoint:                    ep,
			ApplicationServerPrivateKey: pc.ApplicationServerPrivateKey,
			ApplicationServerPublicKey:  pc.ApplicationServerPublicKey,
			Auth:                        pc.Endpoints[ep].Auth,
			P256dh:                      pc.Endpoints[ep].P256dh,
			Payload:                     payload,
		})
		if err != nil {
			log.Infof("Send push request: %v", err)
			continue
		}
		var body [1024]byte
		n, _ := io.ReadFull(resp.Body, body[:])
		resp.Body.Close()
		log.Infof("PUSH RESPONSE %s %s", resp.Status, strings.TrimSpace(string(body[:n])))

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			// TODO: parse retry-after
			// retryAfter := resp.Header.Get("Retry-After")
			pc.Endpoints[ep].RetryAfter = time.Now().Add(5 * time.Minute).Unix()
			changed = true
		} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			delete(pc.Endpoints, ep)
			changed = true
		}
	}
	if changed {
		if err := db.UpdateUser(user); err != nil {
			return err
		}
	}
	return nil
}

// createEmptyPushServiceConfigurationFile creates an empty
// PushServiceConfiguration file.
func (d *Database) createEmptyPushServiceConfigurationFile() error {
	cfg := webpush.DefaultPushServiceConfiguration()
	return d.storage.CreateEmptyFile(d.filePath(pushServiceConfigFile), &cfg)
}

// readPushServiceConfigurationFile reads the push service configuration.
func (d *Database) readPushServiceConfigurationFile() error {
	if err := d.storage.ReadDataFile(d.filePath(pushServiceConfigFile), &d.pushServices); err != nil {
		return err
	}
	return d.pushServices.Init(func(ps *webpush.PushServiceConfiguration) error {
		return d.storage.SaveDataFile(d.filePath(pushServiceConfigFile), ps)
	})
}

// EditPushServiceConfiguration opens an editor for the push service configuration.
func (d *Database) EditPushServiceConfiguration() error {
	var cfg webpush.PushServiceConfiguration
	if err := d.storage.EditDataFile(d.filePath(pushServiceConfigFile), &cfg); err != nil {
		log.Errorf("EditDataFile(%q): %v", d.filePath(pushServiceConfigFile), err)
		return err
	}
	return nil
}
