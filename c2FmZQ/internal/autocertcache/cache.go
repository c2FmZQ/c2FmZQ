package autocertcache

import (
	"context"
	"encoding/base64"

	"golang.org/x/crypto/acme/autocert"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/secure"
)

type cacheContent struct {
	Entries map[string]string `json:"entries"`
}

var _ autocert.Cache = (*Cache)(nil)

// New returns a new Autocert Cache stored in fileName and encrypted with storage.
func New(fileName string, storage *secure.Storage) *Cache {
	storage.CreateEmptyFile(fileName, cacheContent{})
	return &Cache{fileName, storage}
}

// Cache implements autocert.Cache
type Cache struct {
	fileName string
	storage  *secure.Storage
}

// Get returns a cached entry.
func (c *Cache) Get(_ context.Context, key string) ([]byte, error) {
	log.Debugf("Cache.Get(%q)", key)
	var cc cacheContent
	if err := c.storage.ReadDataFile(c.fileName, &cc); err != nil {
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
	commit, err := c.storage.OpenForUpdate(c.fileName, &cc)
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
	commit, err := c.storage.OpenForUpdate(c.fileName, &cc)
	if err != nil {
		return err
	}
	if cc.Entries == nil {
		cc.Entries = make(map[string]string)
	}
	delete(cc.Entries, key)
	return commit(true, nil)
}
