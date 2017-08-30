package peertracker

import (
	"github.com/coreos/etcd/client"
	"golang.org/x/net/context"
	"log"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type peerTracker struct {
	sync.RWMutex
	kv          client.KeysAPI
	id          string
	basePath    string
	ttl         time.Duration
	peers       []string
	peerAddress string
	callback    func([]string)
}

type PeerTracker interface {
	GetPeers() []string
}

func NewPeerTracker(etcdClient client.Client, peerAddress, basePath string, ttl time.Duration, callback func([]string)) PeerTracker {
	kv := client.NewKeysAPI(etcdClient)
	if !strings.HasSuffix(basePath, "/") {
		basePath = basePath + "/"
	}
	regex := regexp.MustCompile("https?://")
	peerId := regex.ReplaceAllString(peerAddress, "")
	tracker := peerTracker{
		id:          peerId,
		peerAddress: peerAddress,
		kv:          kv,
		basePath:    basePath,
		ttl:         ttl,
		callback:    callback,
	}
	tracker.kv.Set(context.Background(), tracker.basePath, "", &client.SetOptions{Dir: true})
	tracker.trackPeers()
	tracker.register()
	tracker.registerShutdown()
	return &tracker
}

func (tracker *peerTracker) GetPeers() []string {
	tracker.RLock()
	res := make([]string, len(tracker.peers))
	copy(res, tracker.peers)
	tracker.RUnlock()
	return res
}

func (tracker *peerTracker) register() {
	go func() {
		for {
			_, err := tracker.kv.Set(
				context.Background(),
				tracker.basePath+tracker.id,
				tracker.peerAddress,
				&client.SetOptions{
					TTL:       tracker.ttl,
					PrevExist: client.PrevExist,
				})
			if err != nil {
				_, err := tracker.kv.Set(
					context.Background(),
					tracker.basePath+tracker.id,
					tracker.peerAddress,
					&client.SetOptions{
						TTL: tracker.ttl,
					})
				if err != nil {
					log.Fatalln("Unable to update etcd, shutting down", err)
				}
			}
			time.Sleep(tracker.ttl / 2)
		}
	}()
}

func (tracker *peerTracker) trackPeers() {
	watcher := tracker.kv.Watcher(tracker.basePath, &client.WatcherOptions{Recursive: true})
	// initialize
	resp, err := tracker.kv.Get(context.Background(), tracker.basePath, nil)
	if err != nil {
		log.Fatalln("Unable to initialize peer list", err)
	}
	if resp.Node.Dir == false {
		log.Fatalln("Not a directory")
	}

	tracker.Lock()
	for _, node := range resp.Node.Nodes {
		if node.Dir == false {
			tracker.peers = append(tracker.peers, node.Value)
		}
	}
	sort.Strings(tracker.peers)
	tracker.Unlock()
	// tracker.callback will be called on register()
	log.Println("Initializing with peers:", tracker.peers)

	go func() {
		for {
			resp, err := watcher.Next(context.Background())
			if err != nil {
				log.Panic(err)
			}
			switch resp.Action {
			case "set":
				{

					// defend against initialization race condition
					tracker.RLock()
					found := false
					for _, node := range tracker.peers {
						if node == resp.Node.Value {
							found = true
						}
					}
					tracker.RUnlock()

					if found == false {
						tracker.Lock()
						tracker.peers = append(tracker.peers, resp.Node.Value)
						sort.Strings(tracker.peers)
						tracker.Unlock()
						tracker.runCallback()
					}
				}
			case "update":
				{
					tracker.addPeer(resp.Node.Value)
				}
			case "expire":
				{
					tracker.deletePeer(resp.PrevNode.Value)
				}
			case "delete":
				{
					tracker.deletePeer(resp.PrevNode.Value)
				}
			default:
				{
					log.Println("Unknown resp", resp)
				}
			}
		}
	}()
}

func (tracker *peerTracker) addPeer(peer string) {
	tracker.RLock()
	found := false
	for _, j := range tracker.peers {
		if j == peer {
			found = true
		}
	}
	tracker.RUnlock()
	if found == false {
		tracker.Lock()
		tracker.peers = append(tracker.peers, peer)
		sort.Strings(tracker.peers)
		tracker.Unlock()
		tracker.runCallback()
	}
}

func (tracker *peerTracker) deletePeer(peer string) {
	tracker.Lock()
	newPeers := []string{}
	log.Println("Testing", peer)
	for _, j := range tracker.peers {
		if j != peer {
			log.Println("Keeping", j)
			newPeers = append(newPeers, j)
		} else {
			log.Println("Removing:", peer)
		}
	}
	tracker.peers = newPeers
	log.Println("Peers after remove:", tracker.peers)
	tracker.Unlock()
	tracker.runCallback()
}

func (tracker *peerTracker) runCallback() {
	tracker.RLock()
	// we use defer since callback might panic
	defer tracker.RUnlock()
	tracker.callback(tracker.peers)
}

// TODO work out what to do with this, forcing os.Exit() is not ideal
func (tracker *peerTracker) registerShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range c {
			_, err := tracker.kv.Delete(context.Background(), tracker.basePath+tracker.id, nil)
			if err != nil {
				log.Println(err)
			}
			os.Exit(0)
		}
	}()
}
