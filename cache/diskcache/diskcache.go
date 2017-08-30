package diskcache

import (
	"container/heap"
	"errors"
	"io"
	"log"
	"os"
	"path"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"strings"
)

type Cache interface {
	Get(key string) (io.ReadCloser, error)
	GetRange(key string, offset, length int64) (io.ReadCloser, error)
	Hit(key string) error
	Put(key string, writer io.Reader) error
	Remove(key string)
	Shutdown() error

	GetFile(key string) (*os.File, error)
}

func New(root string, maxSize int64, cleanedSize int64) (Cache, error) {
	cacheDBPath := path.Join(root, "cache.db")
	db, err := bolt.Open(cacheDBPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Panic("Unable to create or open cache.db", err)
	}
	dc := &diskCache{
		db:          db,
		maxSize:     maxSize,
		cleanedSize: cleanedSize,
		root:        root,
		size:        int64(0),
		dblock:      new(sync.RWMutex),
		fslock:      new(sync.RWMutex),
	}
	dc.fixSize()
	//log.Println("Disk Cache Size:", dc.size)
	//log.Println("Cleaning keys...")
	dc.clean()
	//log.Println("Disk Cache Size:", dc.size)
	//log.Println(dc.cleanedSize)
	//log.Println("Disk Cache Size:", dc.size)
	//log.Println(dc.maxSize)
	//log.Println("Disk Cache Size:", dc.size)
	return dc, nil
}

type diskCache struct {
	cleanedSize int64
	maxSize     int64
	root        string
	size        int64

	db     *bolt.DB
	dblock *sync.RWMutex
	fslock *sync.RWMutex
}

type entry struct {
	key     string
	lastHit time.Time
}

func (dc *diskCache) Get(key string) (io.ReadCloser, error) {
	dc.Hit(key)
	dc.fslock.RLock()
	fi, err := os.Stat(path.Join(dc.root, key))
	dc.fslock.RUnlock()
	if err != nil {
		return nil, err
	}
	// HIT, return full range
	return dc.GetRange(key, 0, fi.Size())
}

func (dc *diskCache) GetRange(key string, offset, length int64) (io.ReadCloser, error) {
	dc.Hit(key)
	key = path.Join(dc.root, key)
	dc.fslock.RLock()
	defer dc.fslock.RUnlock()
	_, err := os.Stat(key)
	if err != nil {
		return nil, err
	}
	reader, writer := io.Pipe()
	go func() {
		dc.fslock.RLock()
		defer dc.fslock.RUnlock()
		file, err := os.Open(key)
		if err != nil {
			io.Copy(writer, file)
			writer.CloseWithError(err)
		}
		defer file.Close()
		file.Seek(offset, 0)
		_, err = io.CopyN(writer, file, length)
		if err != nil {
			writer.CloseWithError(err)
		} else {
			writer.Close()
		}
	}()
	return reader, nil
}

func (dc *diskCache) GetFile(key string) (*os.File, error) {
	dc.Hit(key)
	key = path.Join(dc.root, key)
	return os.Open(key)
}

func (dc *diskCache) Put(key string, reader io.Reader) error {
	dc.fslock.Lock()
	defer dc.fslock.Unlock()
	key = path.Join(dc.root, key)
	file, err := os.OpenFile(key, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, err := io.Copy(file, reader)
	if err != nil {
		file.Close()
		os.RemoveAll(key)
		return err
	}
	dc.size = dc.size + n
	dc.dblock.Lock()
	dc.db.Update(updateKeyTimestamp(key))
	dc.dblock.Unlock()
	dc.clean()
	return nil
}

func (dc *diskCache) Hit(key string) error {
	dc.dblock.Lock()
	defer dc.dblock.Unlock()
	dc.db.Update(updateKeyTimestamp(key))
	return nil
}

func (dc *diskCache) Shutdown() error {
	return errors.New("Not Implemented")
}

type entryHeap []entry

func (h entryHeap) Len() int            { return len(h) }
func (h entryHeap) Less(i, j int) bool  { return h[i].lastHit.Before(h[j].lastHit) }
func (h entryHeap) Swap(i, j int)       {
	h[i], h[j] = h[j], h[i]
}
func (h *entryHeap) Push(x interface{}) { *h = append(*h, x.(entry)) }
func (h *entryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (dc *diskCache) fixSize() {
	keys := &entryHeap{}
	heap.Init(keys)
	dc.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("key-timestamps"))
		if bucket == nil {
			return nil
		}
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var t time.Time
			err := t.UnmarshalBinary(v)
			if err == nil {
				key := entry{
					key:     string(k),
					lastHit: t,
				}
				heap.Push(keys, key)
			}
		}
		return nil
	})
	totalSize := int64(0)
	for keys.Len() > 0 {
		key := keys.Pop().(entry)
		info, err := os.Stat(path.Join(dc.root, key.key))
		if err == nil {
			totalSize = totalSize + info.Size()
		} else if os.IsNotExist(err) {
			dc.db.Update(remove(key.key))
		}
	}
	//log.Println("totalSize", totalSize)
	dc.size = totalSize
}

func (dc *diskCache) clean() {
	//log.Println("clean called")
	keys := &entryHeap{}
	heap.Init(keys)
	dc.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("key-timestamps"))
		if bucket == nil {
			return nil
		}
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var t time.Time
			err := t.UnmarshalBinary(v)
			if err == nil {
				key := entry{
					key:     string(k),
					lastHit: t,
				}
				heap.Push(keys, key)
			}
		}
		return nil
	})

	//log.Println(dc.size, dc.cleanedSize)
	//log.Println(keys)

	for dc.size > dc.cleanedSize {
		//log.Println("cleaning: ", dc.size, ">", dc.cleanedSize)
		//log.Println()
		//log.Println("Before ---")
		//log.Println(keys)
		//log.Println()
		key := heap.Pop(keys).(entry)
		dc.Remove(key.key)
		//log.Println()
		//log.Println("After --- ")
		//log.Println(keys)
		//log.Println()
	}
}

func (dc *diskCache) Remove(key string) {
	file := key
	if !strings.HasPrefix(key, dc.root) {
		file = path.Join(dc.root, key)
	}
	info, err := os.Stat(file)
	if err != nil {
		log.Println(err)
		return
	}
	err = os.Remove(file)
	if err != nil {
		log.Println(err)
		return
	}
	dc.db.Update(remove(key))
	dc.size = dc.size - info.Size()
}

func updateKeyTimestamp(key string) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		updateTime := time.Now()
		binaryUpdateTime, err := updateTime.MarshalBinary()
		if err != nil {
			return err
		}
		bucket, err := tx.CreateBucketIfNotExists([]byte("key-timestamps"))
		if err != nil {
			return err
		}
		return bucket.Put([]byte(key), binaryUpdateTime)
	}
}

func remove(key string) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("key-timestamps"))
		if err != nil {
			return err
		}
		return bucket.Delete([]byte(key))
	}
}
