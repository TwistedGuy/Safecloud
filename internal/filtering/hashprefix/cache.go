package hashprefix

import (
	"encoding/binary"
	"time"

	"github.com/AdguardTeam/golibs/log"
)

// expirySize is the size of expiry in cacheItem.
const expirySize = 8

// cacheItem represents an item that we will store in the cache.
type cacheItem struct {
	// expiry is the time when cacheItem will expire.
	expiry time.Time

	// hashes is the hashed hostnames.
	hashes []hostnameHash
}

// toCacheItem decodes cacheItem from data.  data must be at least equal to
// expiry size.
func toCacheItem(data []byte) *cacheItem {
	t := time.Unix(int64(binary.BigEndian.Uint64(data)), 0)

	data = data[expirySize:]
	hashes := make([]hostnameHash, len(data)/hashSize)

	for i := 0; i < len(data); i += hashSize {
		var hash hostnameHash
		copy(hash[:], data[i:i+hashSize])
		hashes = append(hashes, hash)
	}

	return &cacheItem{
		expiry: t,
		hashes: hashes,
	}
}

// fromCacheItem encodes cacheItem into data.
func fromCacheItem(item *cacheItem) (data []byte) {
	data = make([]byte, len(item.hashes)*hashSize+expirySize)
	expiry := item.expiry.Unix()
	binary.BigEndian.PutUint64(data[:expirySize], uint64(expiry))

	for _, v := range item.hashes {
		// nolint:looppointer // The subsilce is used for a copy.
		data = append(data, v[:]...)
	}

	return data
}

// findInCache finds hashes in the cache.  If nothing found returns list of
// hashes, prefixes of which will be sent to upstream.
func (c *Checker) findInCache(
	hashes []hostnameHash,
) (found, blocked bool, hashesToRequest []hostnameHash) {
	now := time.Now()

	i := 0
	for _, hash := range hashes {
		// nolint:looppointer // The subsilce is used for a safe cache lookup.
		data := c.cache.Get(hash[:prefixLen])
		if data == nil {
			hashes[i] = hash
			i++

			continue
		}

		item := toCacheItem(data)
		if now.After(item.expiry) {
			hashes[i] = hash
			i++

			continue
		}

		if ok := findMatch(hashes, item.hashes); ok {
			return true, true, nil
		}
	}

	if i == 0 {
		return true, false, nil
	}

	return false, false, hashes[:i]
}

// storeInCache caches hashes.
func (c *Checker) storeInCache(hashesToRequest, respHashes []hostnameHash) {
	hashToStore := make(map[prefix][]hostnameHash)

	for _, hash := range respHashes {
		var pref prefix
		// nolint:looppointer // The subsilce is used for a copy.
		copy(pref[:], hash[:])

		hashToStore[pref] = append(hashToStore[pref], hash)
	}

	for pref, hash := range hashToStore {
		// nolint:looppointer // The subsilce is used for a safe cache lookup.
		c.setCache(pref[:], hash)
	}

	for _, hash := range hashesToRequest {
		// nolint:looppointer // The subsilce is used for a safe cache lookup.
		pref := hash[:prefixLen]
		val := c.cache.Get(pref)
		if val == nil {
			c.setCache(pref, nil)
		}
	}
}

// setCache stores hash in cache.
func (c *Checker) setCache(pref []byte, hashes []hostnameHash) {
	item := &cacheItem{
		expiry: time.Now().Add(c.cacheTime),
		hashes: hashes,
	}

	c.cache.Set(pref, fromCacheItem(item))
	log.Debug("%s: stored in cache: %v", c.svc, pref)
}
