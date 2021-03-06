package httpserver

import (
	"errors"
	"github.com/fkautz/casserole/cache/hydrator"
	"github.com/fkautz/casserole/cache/memorycache"
	"github.com/fkautz/casserole/cmd"
	"github.com/gorilla/mux"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func NewHttpHandler(config cmd.Config, cache hydrator.Cache, blockSize int64) http.Handler {
	return &httpHandler{
		cache:     cache,
		blockSize: blockSize,
		config: config,
	}
}

type httpHandler struct {
	cache     hydrator.Cache
	blockSize int64
	config cmd.Config
}

func (s *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get request url
	vars := mux.Vars(r)
	request := vars["request"]

	//if r.Method == "HEAD" {
	//	w.WriteHeader(200)
	//	return
	//}

	w.Header().Add("X-Cache-Server", "casserole/0.0.1")

	// if not cacheable

	// get object
	cacheEntry, err := s.cache.GetMetadata(request, r.Header)
	canonicalRequest := s.config.MirrorUrl + "/" + request
	if err != nil {
		if err.Error() == "Not Cacheable" || err.Error() == "Chunked" {
			log.Println("MISS", request)
			resp, err := s.cache.ForceGet(canonicalRequest)
			if err != nil {
				log.Println(err)
				w.WriteHeader(404)
				return
			}
			io.Copy(w, resp.Body)
			resp.Body.Close()
			return
		} else {
			w.WriteHeader(404)
			return
		}
	}

	// write object metadata
	w.Header().Add("Accept-Ranges", "bytes")

	for k, v := range cacheEntry.Metadata {
		w.Header().Add(k, v)
	}

	cacheCopy := make(map[string]string)

	for k, v := range cacheEntry.Metadata {
		cacheCopy[k] = v
	}

	if k, ok := cacheCopy["Last-Modified"]; ok {
		cacheCopy["Last-Modified"] = strings.Replace(k, " ", "+", -1)
	}

	if k, ok := cacheCopy["X-Cache-Date-Retrieved"]; ok {
		cacheCopy["X-Cache-Date-Retrieved"] = strings.Replace(k, " ", "+", -1)
	}

	cacheEntry.Metadata = cacheCopy

	// if head, get metadata
	if r.Method == "HEAD" {
		w.WriteHeader(200)
		return
	}

	reader, err := s.cache.Get(request, cacheEntry)
	if reader == nil {
		return
	}

	if w.Header().Get("Content-Length") == "" {
		if reader != nil {
			w.Header().Add("Content-Length", strconv.FormatInt(reader.Size(), 10))
		}
	}

	// server
	//http.ServeContent(w, r, request, time.Now(), io.NewSectionReader(reader, 0, reader.Size()))

	// original
	ranges, err := parseRange(r.Header.Get("Range"), reader.Size())
	if err != nil {
		log.Println(err)
	}
	rangeSize := int64(0)
	for _, ra := range ranges {
		rangeSize += ra.length
	}
	if rangeSize > reader.Size() {
		ranges = nil
	}
	streamReader := gcache.NewLazyReader(reader, int64(0), reader.Size(), s.blockSize)
	if ranges == nil {
		w.WriteHeader(200)
		io.Copy(w, streamReader)
	} else {
		http.ServeContent(w, r, request, time.Now(), streamReader)
	}
}

// from "net/http".httpRange
type httpRange struct {
	start, length int64
}

// from "net/http".parseRange
func parseRange(s string, size int64) ([]httpRange, error) {
	if s == "" {
		return nil, nil // header not present
	}
	const b = "bytes="
	if !strings.HasPrefix(s, b) {
		return nil, errors.New("invalid range")
	}
	var ranges []httpRange
	for _, ra := range strings.Split(s[len(b):], ",") {
		ra = strings.TrimSpace(ra)
		if ra == "" {
			continue
		}
		i := strings.Index(ra, "-")
		if i < 0 {
			return nil, errors.New("invalid range")
		}
		start, end := strings.TrimSpace(ra[:i]), strings.TrimSpace(ra[i+1:])
		var r httpRange
		if start == "" {
			// If no start is specified, end specifies the
			// range start relative to the end of the file.
			i, err := strconv.ParseInt(end, 10, 64)
			if err != nil {
				return nil, errors.New("invalid range")
			}
			if i > size {
				i = size
			}
			r.start = size - i
			r.length = size - r.start
		} else {
			i, err := strconv.ParseInt(start, 10, 64)
			if err != nil || i >= size || i < 0 {
				return nil, errors.New("invalid range")
			}
			r.start = i
			if end == "" {
				// If no end is specified, range extends to end of the file.
				r.length = size - r.start
			} else {
				i, err := strconv.ParseInt(end, 10, 64)
				if err != nil || r.start > i {
					return nil, errors.New("invalid range")
				}
				if i >= size {
					i = size - 1
				}
				r.length = i - r.start + 1
			}
		}
		ranges = append(ranges, r)
	}
	return ranges, nil
}
