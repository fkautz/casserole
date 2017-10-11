package hydrator

import (
	"github.com/fkautz/casserole/cache/sizereaderat"
	"github.com/pquerna/cachecontrol/cacheobject"
	"net/http"
)

type Cache interface {
	Get(url string, cacheEntry *CacheEntry) (sizereaderat.SizeReaderAt, error)
	GetMetadata(url string, clientHeaders http.Header) (*CacheEntry, error)
	ForceGet(url string) (resp *http.Response, err error)
}

type CacheEntry struct {
	ObjectResults *cacheobject.ObjectResults
	Metadata      map[string]string
}

type Hydrator interface {
	Get(url string, offset int64, length int64) ([]byte, error)
	GetMetadata(url string) (*CacheEntry, error)
	ForceGet(url string) (*http.Response, error)
}
