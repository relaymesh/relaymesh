package drivers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/relaymesh/relaymesh/pkg/core"
)

// DynamicPublisherCache caches publishers built from driver config JSON.
type DynamicPublisherCache struct {
	mu   sync.Mutex
	pubs map[string]core.Publisher
}

// NewDynamicPublisherCache creates a new cache.
func NewDynamicPublisherCache() *DynamicPublisherCache {
	return &DynamicPublisherCache{pubs: make(map[string]core.Publisher)}
}

// Publisher returns or initializes a publisher for the given driver config.
func (d *DynamicPublisherCache) Publisher(driverName, configJSON string) (core.Publisher, error) {
	if driverName == "" {
		return nil, errors.New("driver name is required")
	}
	key := cacheKey(driverName, configJSON)
	d.mu.Lock()
	pub, ok := d.pubs[key]
	d.mu.Unlock()
	if ok && pub != nil {
		return pub, nil
	}
	cfg, err := ConfigFromDriver(driverName, configJSON)
	fmt.Println(cfg)
	if err != nil {
		return nil, err
	}
	pub, err = core.NewPublisher(cfg)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.pubs[key] = pub
	d.mu.Unlock()
	return pub, nil
}

// Close closes all cached publishers.
func (d *DynamicPublisherCache) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var err error
	for key, pub := range d.pubs {
		if pub != nil {
			err = errors.Join(err, pub.Close())
		}
		delete(d.pubs, key)
	}
	return err
}

func cacheKey(driverName, configJSON string) string {
	hasher := sha256.New()
	hasher.Write([]byte(strings.ToLower(strings.TrimSpace(driverName))))
	hasher.Write([]byte("|"))
	hasher.Write([]byte(strings.TrimSpace(configJSON)))
	return hex.EncodeToString(hasher.Sum(nil))
}
