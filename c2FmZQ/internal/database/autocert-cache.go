package database

import (
	"c2FmZQ/internal/autocertcache"
)

const (
	cacheFile = "autocert-cache.dat"
)

// AutocertCache returns an Autocert Cache that uses the encrypted storage.
func (d *Database) AutocertCache() *autocertcache.Cache {
	return autocertcache.New(d.filePath(cacheFile), d.storage)
}
