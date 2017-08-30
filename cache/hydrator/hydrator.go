package hydrator

import (
	"crypto/tls"
	"errors"
	"github.com/pquerna/cachecontrol/cacheobject"
	"log"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

func NewHydrator(urlRoot string) Hydrator {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	client.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConns:        0,
		MaxIdleConnsPerHost: 0,
		DisableKeepAlives:   false,
	}
	impl := &hydratorImpl{
		urlRoot: urlRoot,
		client:  client,
	}
	return impl
}

type hydratorImpl struct {
	urlRoot string
	client  *http.Client
}

func (h *hydratorImpl) Get(key string, start int64, end int64) ([]byte, error) {
	url := h.urlRoot + "/" + key
	log.Println("get", url, start, end)

	byteRange := "bytes=" + strconv.FormatInt(start, 10) + "-" + strconv.FormatInt(end-1, 10)

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	//log.Println("Range", byteRange)
	request.Header.Add("Range", byteRange)
	response, err := h.client.Do(request)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	//time.Sleep(1 * time.Second)

	if err != nil {
		return nil, err
	}
	return data, nil
}

func (h *hydratorImpl) ForceGet(url string) (*http.Response, error) {
	return h.client.Get(url)
}

func (h *hydratorImpl) GetMetadata(key string) (*CacheEntry, error) {
	url := h.urlRoot + "/" + key
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	response, err := h.client.Do(request)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	io.Copy(ioutil.Discard, response.Body)
	response.Body.Close()
	//time.Sleep(1 * time.Second)

	if response.StatusCode != http.StatusOK {
		return nil, errors.New("Unexpected status: " + strconv.Itoa(response.StatusCode))
	}
	// log.Println(response.Header)

	metadata := make(map[string]string)
	SetIfNotEmpty(metadata, response.Header, "Content-Encodling")
	SetIfNotEmpty(metadata, response.Header, "Content-Length")
	SetIfNotEmpty(metadata, response.Header, "Content-MD5")
	SetIfNotEmpty(metadata, response.Header, "Content-Type")
	SetIfNotEmpty(metadata, response.Header, "Etag")
	SetIfNotEmpty(metadata, response.Header, "Last-Modified")

	// Always set
	metadata["X-Cache-Date-Retrieved"] = response.Header.Get("Date")

	//metadata["Content-Encoding"] = response.Header.Get("Content-Encoding")
	//metadata["Content-Length"] = size
	//metadata["Content-Md5"] = response.Header.Get("Content-MD5")
	//metadata["Content-Type"] = response.Header.Get("Content-Type")
	//metadata["Etag"] = response.Header.Get("Etag")
	//metadata["Last-Modified"] = response.Header.Get("Last-Modified")

	cacheResults, err := getCacheResult(request, response)
	log.Println(cacheResults)
	if err != nil {
		return nil, err
	}
	//log.Println("h", metadata)
	return &CacheEntry{
		ObjectResults: cacheResults,
		Metadata:      metadata,
	}, nil
}

func SetIfNotEmpty(dest map[string]string, orig http.Header, key string) {
	if key != "" && orig.Get(key) != "" {
		dest[key] = orig.Get(key)
	}
}

func getCacheResult(req *http.Request, res *http.Response) (*cacheobject.ObjectResults, error) {
	reqDir, err := cacheobject.ParseRequestCacheControl(req.Header.Get("Cache-Control"))
	if err != nil {
		return nil, err
	}

	resDir, err := cacheobject.ParseResponseCacheControl(res.Header.Get("Cache-Control"))
	if err != nil {
		return nil, err
	}
	expiresHeader, _ := http.ParseTime(res.Header.Get("Expires"))
	dateHeader, _ := http.ParseTime(res.Header.Get("Date"))
	lastModifiedHeader, _ := http.ParseTime(res.Header.Get("Last-Modified"))

	obj := cacheobject.Object{
		RespDirectives:         resDir,
		RespHeaders:            res.Header,
		RespStatusCode:         res.StatusCode,
		RespExpiresHeader:      expiresHeader,
		RespDateHeader:         dateHeader,
		RespLastModifiedHeader: lastModifiedHeader,

		ReqDirectives: reqDir,
		ReqHeaders:    req.Header,
		ReqMethod:     req.Method,

		NowUTC: time.Now().UTC(),
	}
	rv := cacheobject.ObjectResults{}

	cacheobject.CachableObject(&obj, &rv)
	cacheobject.ExpirationObject(&obj, &rv)
	//log.Println(obj)
	//log.Println(rv)

	if rv.OutErr != nil {
		return nil, rv.OutErr
	}

	//log.Println("Errors: ", rv.OutErr)
	//log.Println("Reasons to not cache: ", rv.OutReasons)
	//log.Println("Warning headers to add: ", rv.OutWarnings)
	//log.Println("Expiration: ", rv.OutExpirationTime.String())
	return &rv, nil
}
