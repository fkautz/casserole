// Copyright © 2016 Frederick F. Kautz IV fkautz@alumni.cmu.edu
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	cmd2 "github.com/fkautz/casserole/cmd"
	"log"
	"net/http"
	"os"

	"code.cloudfoundry.org/bytefmt"
	"github.com/fkautz/casserole/cache/diskcache"
	"github.com/fkautz/casserole/cache/httpserver"
	"github.com/fkautz/casserole/cache/hydrator"
	"github.com/fkautz/casserole/cache/memorycache"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/kelseyhightower/envconfig"
)

var config cmd2.Config

func main() {
	log.SetFlags(log.Flags() | log.Lshortfile)

	err := envconfig.Process("casserole", &config)
	if err != nil {
		log.Fatal(err.Error())
	}

	blockSize := int64(2 * 1024 * 1024)

	var persistentCache diskcache.Cache
	if config.DiskCacheEnabled {
		maxSize, err := bytefmt.ToBytes(config.MaxDiskUsage)
		if err != nil {
			log.Fatalln("Unable to parse max-disk-usage", err)
		}
		cleanedSize, err := bytefmt.ToBytes(config.CleanedDiskUsage)
		if err != nil {
			log.Fatalln("Unable to parse cleaned-disk-usage", err)
		}
		persistentCache, err = diskcache.New(config.DiskCacheDir, int64(maxSize), int64(cleanedSize))
		if err != nil {
			log.Fatalln("Unable to initialize disk cache", err)
		}
	}

	maxMemory, err := bytefmt.ToBytes(config.MaxMemoryUsage)
	if err != nil {
		log.Fatalln("Unable to parse max-memory-usage", err)
	}

	cacheConfig := gcache.Config{
		MaxMemoryUsage: int64(maxMemory),
		BlockSize:      blockSize,
		DiskCache:      persistentCache,
		Hydrator:       hydrator.NewHydrator(config.MirrorUrl),
		PeeringAddress: config.PeeringAddress,
		Etcd:           config.Etcd,
		PassThrough:    config.Passthrough,
	}

	cache := gcache.NewCache(cacheConfig)

	cacheHandler := httpserver.NewHttpHandler(config, cache, blockSize)

	router := mux.NewRouter()

	groupCacheProxyHandler := http.Handler(cacheHandler)
	router.Handle("/{request:.*}", groupCacheProxyHandler)

	// serve
	//err = http.ListenAndServeTLS(address, "cert.pem", "key.pem", router)
	//handler := lox.NewHandler(lox.NewMemoryCache(), router)
	var handler http.Handler
	handler = router
	handler = handlers.LoggingHandler(os.Stderr, handler)
	err = http.ListenAndServe(config.Address, handler)
	if err != nil {
		log.Fatalln(err)
	}

}
