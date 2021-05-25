package database

import (
	"context"
	"encoding/base64"

	"golang.org/x/crypto/acme/autocert"

	"c2FmZQ/internal/log"
)

const (
	cacheFile = "autocert-cache.dat"
)

type cacheContent struct {
	Entries map[string]string `json:"entries"`
}

// AutocertCache returns the an autocert.Cache that uses the encrypted storage.
func (d *Database) AutocertCache() *Cache {
	d.storage.CreateEmptyFile(d.filePath(cacheFile), cacheContent{})
	return &Cache{d}
}

var _ autocert.Cache = (*Cache)(nil)

// Cache implements autocert.Cache
type Cache struct {
	db *Database
}

// Get returns a cached entry.
func (c *Cache) Get(_ context.Context, key string) ([]byte, error) {
	log.Debugf("Cache.Get(%q)", key)
	var cc cacheContent
	if err := c.db.storage.ReadDataFile(c.db.filePath(cacheFile), &cc); err != nil {
		return nil, err
	}
	if cc.Entries == nil {
		cc.Entries = make(map[string]string)
	}
	e, ok := cc.Entries[key]
	if !ok {
		log.Debugf("Cache.Get(%q) NOT found.", key)
		return nil, autocert.ErrCacheMiss
	}
	log.Debugf("Cache.Get(%q) found.", key)
	return base64.StdEncoding.DecodeString(e)
}

// Put stores a cache entry.
func (c *Cache) Put(_ context.Context, key string, data []byte) error {
	log.Debugf("Cache.Put(%q, ...)", key)
	var cc cacheContent
	commit, err := c.db.storage.OpenForUpdate(c.db.filePath(cacheFile), &cc)
	if err != nil {
		return err
	}
	if cc.Entries == nil {
		cc.Entries = make(map[string]string)
	}
	cc.Entries[key] = base64.StdEncoding.EncodeToString(data)
	return commit(true, nil)
}

// Delete deletes a cached entry.
func (c *Cache) Delete(_ context.Context, key string) error {
	log.Debugf("Cache.Delete(%q)", key)
	var cc cacheContent
	commit, err := c.db.storage.OpenForUpdate(c.db.filePath(cacheFile), &cc)
	if err != nil {
		return err
	}
	if cc.Entries == nil {
		cc.Entries = make(map[string]string)
	}
	delete(cc.Entries, key)
	return commit(true, nil)
}
