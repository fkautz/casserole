package gcache

import (
	"bytes"
	"encoding/gob"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/fkautz/casserole/cache/hydrator"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"sync"
	"time"
)

type MetadataSyncer interface {
	Add(key string, value hydrator.CacheEntry) error
	Remove(key string) error
	Sync()
}

type metadataSync struct {
	cache  MetadataCache
	client *clientv3.Client
}

func NewMetadataSyncer(cache MetadataCache, c *clientv3.Client) error {
	syncer := &metadataSync{
		cache:  cache,
		client: c,
	}

	cache.AddSync(syncer)
	go syncer.Sync()
	return nil
}

func (syncer *metadataSync) Add(key string, value hydrator.CacheEntry) error {
	kv := clientv3.NewKV(syncer.client)
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	encoder.Encode(value)
	//log.Println(value)
	//log.Println(buf.String())
	duration := value.ObjectResults.OutExpirationTime.Sub(time.Now())
	ttlInSeconds := int64(duration / time.Second)
	//log.Println(key+" TTL:", ttlInSeconds)
	leaseResp, err := syncer.client.Lease.Grant(context.Background(), ttlInSeconds)
	if err != nil {
		return err
	}
	_, err = kv.Put(context.TODO(), key, string(buf.Bytes()), clientv3.WithLease(leaseResp.ID))
	if err != nil {
		return err
	}
	return err
}

func (syncer *metadataSync) Remove(key string) error {
	kv := clientv3.NewKV(syncer.client)
	_, err := kv.Delete(context.Background(), key)
	if err != nil {
		return err
	}
	return nil
}

func (syncer *metadataSync) Sync() {
	// set up etcd
	watcher := clientv3.NewWatcher(syncer.client)
	ch := watcher.Watch(context.Background(), "", clientv3.WithPrefix())
	for response := range ch {
		for _, event := range response.Events {
			switch event.Type {
			case mvccpb.PUT:
				decoder := gob.NewDecoder(bytes.NewBuffer(event.Kv.Value))
				value := hydrator.CacheEntry{}
				decoder.Decode(&value)
				//log.Println("Sync PUT", string(event.Kv.Key), value)
				syncer.cache.AddWithoutSync(string(event.Kv.Key), value)
			case mvccpb.DELETE:
				log.Println("Sync DELETE", string(event.Kv.Key))
				syncer.cache.RemoveWithoutSync(string(event.Kv.Key))
			default:
				log.Println("Sync Unknown Type")
			}
		}
	}
}

type MetadataCache interface {
	Add(key string, cacheEntry hydrator.CacheEntry) error
	AddWithoutSync(key string, metadata hydrator.CacheEntry)
	Get(key string, clientHeaders http.Header) (*hydrator.CacheEntry, bool)
	Remove(key string)
	RemoveWithoutSync(key string)
	AddSync(syncer MetadataSyncer)
}

type metadataCache struct {
	metadata map[string]hydrator.CacheEntry
	lock     sync.RWMutex
	syncer   MetadataSyncer
}

func NewMetadataCache() MetadataCache {
	return &metadataCache{
		// Object metadata cache [key: [header: value]]
		metadata: make(map[string]hydrator.CacheEntry),
	}
}

func (cache *metadataCache) Add(key string, metadata hydrator.CacheEntry) error {
	err := cache.syncer.Add(key, metadata)
	if err != nil {
		return err
	}
	cache.add(key, metadata)
	return nil
}

func (cache *metadataCache) AddWithoutSync(key string, cacheEntry hydrator.CacheEntry) {
	cache.add(key, cacheEntry)
}

func (cache *metadataCache) Get(key string, clientHeaders http.Header) (*hydrator.CacheEntry, bool) {
	cache.lock.RLock()
	res, ok := cache.metadata[key]
	cache.lock.RUnlock()
	return &res, ok
}

func (cache *metadataCache) Remove(key string) {
	cache.lock.Lock()
	delete(cache.metadata, key)
	cache.lock.Unlock()
}

func (cache *metadataCache) RemoveWithoutSync(key string) {
	cache.lock.Lock()
	cache.syncer.Remove(key)
	delete(cache.metadata, key)
	cache.lock.Unlock()
}

func (cache *metadataCache) add(key string, cacheEntry hydrator.CacheEntry) {
	cache.lock.Lock()
	cache.metadata[key] = cacheEntry
	cache.lock.Unlock()
}

func (cache *metadataCache) AddSync(syncer MetadataSyncer) {
	cache.syncer = syncer
}
