# Casserole

Casserole is a distributed http cache with a focus on:

* Large Objects
* Bandwidth minimization
* Multi-tier distributed cache sharing
  * Local Memory
  * Cluster Memory
  * Cluster Persistent Storage
* No single point of failure

Casserole is written in Go and integrates well-tested distributed libraries such as etcd and groupcache.

# Upstream Server Consideratoins

### HTTP headers must be set to allow caching.

* HTTP headers are used to determine cacheability.
* Cacheable objects expiring in less than 60 seconds are not cached.
* The HTTP verb `HEAD` is used to determine whether an object is cacheable, not `GET`.
* Responses are immediately streamed if the object is not cached.
* Upstream server must allow `Range` requests on cacheable objects.
* The cluster will download cacheable large objects in `2 megabyte` intervals and will deliver each interval as soon as
it is received.

### HTTP Range required

Upstream objects are fetched in `2MB` segments. Objects must support fetching objects through `Range: bytes` requests.

When retrieving objects larger than 2MB, each Range request to the same object must return the same object, or the request
will be corrupted. For large objects, this is typically not a concern.

### Unauthenticated requests

At the moment, only unauthenticated requests are supported. Authenticated requests will be supported at
a later time.

# Getting Started

Casserole is still in an alpha development state. Production use at this time is
not recommended. Feedback, testing and help are greatly appreciated.

## Single Node Testing Environment

Single node testing is supported with docker-compose. The `docker-compose.yml`
file is located at `utils/compose` and can be started with the following command:

`docker-compose up`

To scale the cache, use

`docker-compose scale cache=3`

There are multiple configuration options available for the test environment that can
be configured in the `docker-compose.yml` file.

Here are the configurable environment variables with their default values:
```sh
UPSTREAM_SERVER http://www.example.com
ETCD http://etcd:2379
MAX_MEMORY 100m
MAX_DISK 100G
````

## Multi Node Setup

Prerequisites:

* Set up an etcd cluster
* Determine how much memory you want to use.
* Determine how much disk space you want to use.

The prefered technique for starting casserole is with docker:

```sh
docker pull fkautz/casserole
docker run -d \
           -e UPSTREAM_SERVER=http://httpbin.org  \
           -e ETCD=http://etcd:2379 \
           fkautz/casserole
```

Configurable options when running casserole:

```sh
      --address string              Address to listen on (default "localhost:8080")
      --cleaned-disk-usage string   Address to listen on (default "800M")
      --disk-cache-dir string       Address to listen on (default "./data")
      --disk-cache-enabled          Address to listen on (default true)
      --etcd value                  URL root to mirror (default [])
      --max-disk-usage string       Address to listen on (default "1G")
      --max-memory-usage string     Address to listen on (default "100M")
      --mirror-url string           URL root to mirror (default "http://localhost:9000")
      --peering-address string      URL root to mirror (default "http://localhost:8000")
```

# Reporting Feature Requests and Bugs

Please file all bugs and feature requests to `https://github.com/fkautz/casserole/issues`.

Pull requests are welcome. For new features or complex pull requests, please contact `fkautz`
first. He can be reached at `#casserole` on `irc.freenode.net`.
