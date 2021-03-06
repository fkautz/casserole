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
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// flags
var (
	diskCacheEnabled bool
	maxDiskUsage     string
	maxMemoryUsage   string
	mirrorUrl        string
	peeringAddress   string
	etcd             []string
	passthrough      []string
)

type Config struct {
	Address string `default:"localhost:8080"`
	CleanedDiskUsage string `default:"800M"`
	DiskCacheDir string `default:"./data"`
}

var config Config

func InitializeConfig(cmd *cobra.Command) {
	err := envconfig.Process("casserole", &config)
	if err != nil {
		log.Fatal(err.Error())
	}
	viper.SetDefault("disk-cache-enabled", true)
	viper.SetDefault("max-disk-usage", "1G")
	viper.SetDefault("max-memory-usage", "100M")
	viper.SetDefault("mirror-url", "http://localhost:9000")
	viper.SetDefault("peering-address", "")
	viper.SetDefault("etcd", "")
	viper.SetDefault("passthrough", "")

	if cmd2.FlagChanged(cmd.PersistentFlags(), "disk-cache-enabled") {
		viper.Set("disk-cache-enabled", diskCacheEnabled)
	}
	if cmd2.FlagChanged(cmd.PersistentFlags(), "max-disk-usage") {
		viper.Set("max-disk-usage", maxDiskUsage)
	}
	if cmd2.FlagChanged(cmd.PersistentFlags(), "max-meory-usage") {
		viper.Set("max-memory-usage", maxMemoryUsage)
	}
	if cmd2.FlagChanged(cmd.PersistentFlags(), "mirror-url") {
		viper.Set("mirror-url", mirrorUrl)
	}
	if cmd2.FlagChanged(cmd.PersistentFlags(), "peering-address") {
		viper.Set("peering-address", peeringAddress)
	}
	if cmd2.FlagChanged(cmd.PersistentFlags(), "etcd") {
		viper.Set("etcd", etcd)
	}
	if cmd2.FlagChanged(cmd.PersistentFlags(), "passthrough") {
		viper.Set("passthrough", passthrough)
	}
}

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		log.SetFlags(log.Flags() | log.Lshortfile)

		InitializeConfig(cmd)
		blockSize := int64(2 * 1024 * 1024)

		var persistentCache diskcache.Cache
		if viper.GetBool("disk-cache-enabled") {
			maxSize, err := bytefmt.ToBytes(viper.GetString("max-disk-usage"))
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

		maxMemory, err := bytefmt.ToBytes(viper.GetString("max-memory-usage"))
		if err != nil {
			log.Fatalln("Unable to parse max-memory-usage", err)
		}

		cacheConfig := gcache.Config{
			MaxMemoryUsage: int64(maxMemory),
			BlockSize:      blockSize,
			DiskCache:      persistentCache,
			Hydrator:       hydrator.NewHydrator(viper.GetString("mirror-url")),
			PeeringAddress: viper.GetString("peering-address"),
			Etcd:           viper.GetStringSlice("etcd"),
			PassThrough:    viper.GetStringSlice("passthrough"),
		}

		cache := gcache.NewCache(cacheConfig)

		cacheHandler := httpserver.NewHttpHandler(cache, blockSize)

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
	},
}

type appConfig struct {
	urlRoot  string
	listenOn string
}

func init() {
	cmd2.RootCmd.AddCommand(serverCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serverCmd.PersistentFlags().String("foo", "", "A help for foo")

	serverCmd.PersistentFlags().StringVar(&maxMemoryUsage, "max-memory-usage", "100M", "Address to listen on")
	serverCmd.PersistentFlags().StringVar(&maxDiskUsage, "max-disk-usage", "1G", "Address to listen on")
	serverCmd.PersistentFlags().BoolVar(&diskCacheEnabled, "disk-cache-enabled", true, "Address to listen on")
	serverCmd.PersistentFlags().StringVar(&mirrorUrl, "mirror-url", "http://localhost:9000", "URL root to mirror")
	serverCmd.PersistentFlags().StringVar(&peeringAddress, "peering-address", "http://localhost:8000", "URL root to mirror")
	serverCmd.PersistentFlags().StringSliceVar(&etcd, "etcd", []string{}, "URL root to mirror")
	serverCmd.PersistentFlags().StringSliceVar(&passthrough, "passthrough", []string{}, "Regexes to ignore")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serverCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

}

func main() {
	if err := cmd2.RootCmd.Execute(); err != nil {
		log.Panicln(err)
	}
}
