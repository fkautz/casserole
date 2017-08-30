package gcache

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/clientv3"
	"github.com/fkautz/peertracker"
	"github.com/fkautz/casserole/cache/diskcache"
	"github.com/fkautz/casserole/cache/hydrator"
	"github.com/fkautz/casserole/cache/sizereaderat"
	"github.com/golang/groupcache"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MetadataRequest struct {
	Url     string
	Key     string
	Headers map[string]string
}

type dataRequest struct {
	MetadataRequest
	Block     int64
	Size      int64
	BlockSize int64
}

type cacheContext struct {
	diskCache diskcache.Cache
	hydrator  hydrator.Hydrator
}

type memoryCache struct {
	group            *groupcache.Group
	diskCache        diskcache.Cache
	hydrator         hydrator.Hydrator
	blockSize        int64
	groupName        string
	metadata         MetadataCache
	passthroughRegex *regexp.Regexp
}

type Config struct {
	MaxMemoryUsage int64
	BlockSize      int64
	DiskCache      diskcache.Cache
	Hydrator       hydrator.Hydrator
	GroupName      string
	PeeringAddress string
	Etcd           []string
	PassThrough    []string
}

type NotCacheable struct{}

func (_ NotCacheable) Error() string {
	return "Not Cacheable"
}

type Key struct {
	// required
	Url string

	// required if present
	ContentLength   uint64 `json:",omitempty"`
	ContentEncoding string `json:",omitempty"`

	// require only first.
	// last retrieved used as fallback
	// if last retrieved is set, always set to time.Now() to force expiration
	Etag          string `json:",omitempty"`
	Sha512        string `json:",omitempty"`
	Sha256        string `json:",omitempty"`
	Sha1          string `json:",omitempty"`
	Md5           string `json:",omitempty"`
	LastModified  string `json:",omitempty"`
	LastRetrieved string `json:",omitempty"`
}

func GenerateKey(url string, headers map[string]string) ([]byte, error) {
	key := Key{
		Url: url,
	}

	normalizedHeaders := make(map[string]string)
	for k, v := range headers {
		normalizedHeaders[strings.ToLower(k)] = v
	}

	if v, ok := normalizedHeaders["content-length"]; ok == true {
		if length, err := strconv.ParseUint(v, 10, 64); err == nil {
			key.ContentLength = length
		}
	}

	if v, ok := normalizedHeaders["content-encoding"]; ok == true {
		key.ContentEncoding = v
	}

	var lastRetrieved string
	dateString, ok := normalizedHeaders["x-date-retrieved"]
	var lastRetrievedError error
	if ok == true {
		var lastRetrievedTime time.Time
		lastRetrievedTime, lastRetrievedError = http.ParseTime(dateString)
		if lastRetrievedError == nil {
			lastRetrieved = lastRetrievedTime.UTC().String()
		}
	}

	if v, ok := normalizedHeaders["etag"]; ok == true {
		key.Etag = v
	} else if v, ok := headers["sha512"]; ok == true {
		key.Etag = v
	} else if v, ok := headers["sha256"]; ok == true {
		key.Etag = v
	} else if v, ok := headers["sha1"]; ok == true {
		key.Etag = v
	} else if v, ok := headers["content-md5"]; ok == true {
		key.Etag = v
	} else if v, ok := headers["last-modified"]; ok == true {
		lastModifiedTime, err := http.ParseTime(v)
		if err == nil {
			key.LastModified = lastModifiedTime.UTC().String()
		} else {
			if lastRetrievedError != nil {
				return nil, lastRetrievedError
			}
			key.LastRetrieved = lastRetrieved
		}
	} else {
		// error was generated before if statements
		if lastRetrievedError != nil {
			return nil, lastRetrievedError
		}
		key.LastRetrieved = lastRetrieved
	}

	js, err := json.Marshal(key)
	if err != nil {
		return nil, err
	}
	shasum := sha256.Sum256(js)

	return shasum[:], nil
}
func (mc *memoryCache) GetMetadata(url string, clientHeaders http.Header) (*hydrator.CacheEntry, error) {

	if mc.passthroughRegex != nil {
		if mc.passthroughRegex.MatchString(url) {
			return nil, NotCacheable{}
		}
	}

	var cacheEntry *hydrator.CacheEntry
	cacheEntry, foundMetadata := mc.metadata.Get(url, clientHeaders)

	var err error
	if !foundMetadata {
		cacheEntry, err = mc.hydrator.GetMetadata(url)
		if err != nil {
			return nil, err
		}

		if len(cacheEntry.ObjectResults.OutReasons) > 0 {
			return nil, NotCacheable{}
		}

		//now := time.Now()
		//exp := cacheEntry.ObjectResults.OutExpirationTime
		//log.Println("Now:", now)
		//log.Println("Exp:", exp)
		if cacheEntry.ObjectResults.OutExpirationTime.Before(time.Now().Add(60 * time.Second)) {
			//log.Println("SKIP")
			return nil, NotCacheable{}
		} else if v, ok := cacheEntry.Metadata["Accept-Ranges"]; ok == true {
			if strings.ToLower(string(v[0])) == "none" {
				return nil, NotCacheable{}
			}
		} else {
			//log.Println("CACHE")
		}

		if err := mc.metadata.Add(url, *cacheEntry); err != nil {
			return nil, err
		}
	}
	return cacheEntry, nil
}

func (mc *memoryCache) Get(url string, cacheEntry *hydrator.CacheEntry) (sizereaderat.SizeReaderAt, error) {

	// Just passing headers in naively
	sum, err := GenerateKey(url, cacheEntry.Metadata)
	key := hex.EncodeToString(sum[:])
	metadataRequest := MetadataRequest{
		Url:     url,
		Key:     key,
		Headers: cacheEntry.Metadata,
	}

	ctx := cacheContext{
		diskCache: mc.diskCache,
		hydrator:  mc.hydrator,
	}

	totalSize, err := strconv.ParseInt(cacheEntry.Metadata["Content-Length"], 10, 64)
	if err != nil {
		return nil, err
	}

	// TODO blockCount
	blockCount := int(totalSize/mc.blockSize + 1)

	var parts []sizereaderat.SizeReaderAt
	sizeLeft := totalSize
	for i := 0; i < blockCount; i++ {
		request := dataRequest{
			MetadataRequest: metadataRequest,
			Block:           int64(i),
			Size:            totalSize,
			BlockSize:       mc.blockSize,
		}
		partSize := mc.blockSize
		if sizeLeft < partSize {
			partSize = sizeLeft
		}
		part := lazyReaderAt{
			request:   request,
			size:      partSize,
			groupName: mc.groupName,
			ctx:       ctx,
		}
		sizeLeft = sizeLeft - part.size
		//go part.ReadAt(make([]byte, 1), 0) // Preload cache
		parts = append(parts, part)
	}

	unalignedReader := sizereaderat.NewMultiReaderAt(parts...)
	//alignedReader := sizereaderat.NewChunkAlignedReaderAt(unalignedReader, int(mc.blockSize))
	return unalignedReader, nil
}

func (mc *memoryCache) ForceGet(url string) (resp *http.Response, err error) {
	return mc.hydrator.ForceGet(url)
}

func (mc *memoryCache) getRange(url string, offset int64, length int64) (io.ReaderAt, error) {
	return nil, errors.New("Not Implemented")
}

var setupPool = sync.Once{}

func NewCache(config Config) hydrator.Cache {
	setupPool.Do(func() {
		me := "http://127.0.0.1:8000"
		regex := regexp.MustCompile("https?://")
		if config.PeeringAddress != "" {
			me = config.PeeringAddress
		}
		addr := regex.ReplaceAllString(me, "")
		peers := groupcache.NewHTTPPool(me)
		peers.Context = func(req *http.Request) groupcache.Context {
			return cacheContext{
				diskCache: config.DiskCache,
				hydrator:  config.Hydrator,
			}
		}
		etcdConfig := client.Config{
			Endpoints: config.Etcd,
			//Transport:               client.DefaultTransport,
			//HeaderTimeoutPerRequest: time.Second,
		}
		etcdClient, err := client.New(etcdConfig)
		if err != nil {
			log.Fatalln("Could not connect to etcd")
		}
		time.Sleep(2 * time.Second)
		peertracker.NewPeerTracker(etcdClient, me, "/casserole/peers", 60*time.Second, func(newPeers []string) {
			log.Println("Settings peers:", newPeers)
			peers.Set(newPeers...)
		})
		go func() {
			handler := peers
			//handler = handlers.LoggingHandler(os.Stderr, peers)
			if err := http.ListenAndServe(addr, handler); err != nil {
				log.Panicln(err)
			}
		}()
	})

	if config.GroupName == "" {
		config.GroupName = "default"
	}

	group := groupcache.NewGroup(config.GroupName, config.MaxMemoryUsage, groupcache.GetterFunc(getterFunc))

	etcdConfig := clientv3.Config{
		Endpoints: config.Etcd,
	}

	etcdClientV3, err := clientv3.New(etcdConfig)
	if err != nil {
		log.Panicln(err)
	}

	mdCache := NewMetadataCache()
	NewMetadataSyncer(mdCache, etcdClientV3)
	log.Println("Passthrough:")

	var passthroughRegex *regexp.Regexp
	if len(config.PassThrough) > 0 {
		passThroughRegexString := strings.Join(config.PassThrough, "|")
		passthroughRegex = regexp.MustCompile(passThroughRegexString)
	}

	mc := &memoryCache{
		group:            group,
		diskCache:        config.DiskCache,
		hydrator:         config.Hydrator,
		blockSize:        config.BlockSize,
		groupName:        config.GroupName,
		metadata:         mdCache,
		passthroughRegex: passthroughRegex,
	}

	return mc
}

type mcContext struct{}

func getterFunc(ctx groupcache.Context, key string, dest groupcache.Sink) error {
	typedCtx := ctx.(cacheContext)

	dataRegex, err := regexp.Compile("^data\\/")
	metadataRegex, err := regexp.Compile("^metadata\\/")
	if metadataRegex.MatchString(key) {
		key = key[9:]
		info := MetadataRequest{}
		err = json.Unmarshal([]byte(key), &info)
		if err != nil {
			return err
		}
		cacheEntry, err := typedCtx.hydrator.GetMetadata(info.Url)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		encoder := gob.NewEncoder(&buf)
		encoder.Encode(cacheEntry)
		dest.SetBytes(buf.Bytes())
		return nil
	} else if dataRegex.MatchString(key) {
		key = key[5:]
		info := dataRequest{}
		err = json.Unmarshal([]byte(key), &info)
		if err != nil {
			return err
		} // read from disk
		start := info.Block * info.BlockSize
		end := (info.Block + 1) * info.BlockSize
		if info.Size < end {
			end = info.Size
		}
		diskKey := info.Key + "-" + strconv.FormatInt(info.Block, 10)
		reader, err := typedCtx.diskCache.Get(diskKey)
		if err == nil {
			data, err := ioutil.ReadAll(reader)
			if err == nil {
				dest.SetBytes(data)
				return nil
			}
		}

		// if not on disk, hydrate from upstream and store to disk
		data, err := typedCtx.hydrator.Get(info.Url, start, end)
		if err != nil {
			return err
		}
		err = typedCtx.diskCache.Put(diskKey, bytes.NewBuffer(data))
		if err != nil {
			return err
		}
		dest.SetBytes(data)
		return nil
	} else {
		return errors.New("Unknown type request")
	}
}
