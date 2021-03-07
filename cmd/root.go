// Copyright © 2016 NAME HERE fkautz@alumni.cmu.edu
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

package cmd

type Config struct {
	Address string `default:"localhost:8080"`
	CleanedDiskUsage string `default:"800M"`
	DiskCacheDir string `default:"./data"`
	DiskCacheEnabled bool `default:"true"`
	MaxDiskUsage string `default:"1G"`
	MaxMemoryUsage string `default:"100M"`
	MirrorUrl string `default:"http://localhost:9000"`
	PeeringAddress string `default"http://localhost:8000"`
	Etcd []string `default:""`
	Passthrough []string `default:""`
}