// Copyright 2024 Kelvin Clement Mwinuka
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/pkg/constants"
	"log"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	TLS                bool          `json:"TLS" yaml:"TLS"`
	MTLS               bool          `json:"MTLS" yaml:"MTLS"`
	CertKeyPairs       [][]string    `json:"CertKeyPairs" yaml:"CertKeyPairs"`
	ClientCAs          []string      `json:"ClientCAs" yaml:"ClientCAs"`
	Port               uint16        `json:"Port" yaml:"Port"`
	ServerID           string        `json:"ServerId" yaml:"ServerId"`
	JoinAddr           string        `json:"JoinAddr" yaml:"JoinAddr"`
	BindAddr           string        `json:"BindAddr" yaml:"BindAddr"`
	RaftBindPort       uint16        `json:"RaftPort" yaml:"RaftPort"`
	MemberListBindPort uint16        `json:"MlPort" yaml:"MlPort"`
	InMemory           bool          `json:"InMemory" yaml:"InMemory"`
	DataDir            string        `json:"DataDir" yaml:"DataDir"`
	BootstrapCluster   bool          `json:"BootstrapCluster" yaml:"BootstrapCluster"`
	AclConfig          string        `json:"AclConfig" yaml:"AclConfig"`
	ForwardCommand     bool          `json:"ForwardCommand" yaml:"ForwardCommand"`
	RequirePass        bool          `json:"RequirePass" yaml:"RequirePass"`
	Password           string        `json:"Password" yaml:"Password"`
	SnapShotThreshold  uint64        `json:"SnapshotThreshold" yaml:"SnapshotThreshold"`
	SnapshotInterval   time.Duration `json:"SnapshotInterval" yaml:"SnapshotInterval"`
	RestoreSnapshot    bool          `json:"RestoreSnapshot" yaml:"RestoreSnapshot"`
	RestoreAOF         bool          `json:"RestoreAOF" yaml:"RestoreAOF"`
	AOFSyncStrategy    string        `json:"AOFSyncStrategy" yaml:"AOFSyncStrategy"`
	MaxMemory          uint64        `json:"MaxMemory" yaml:"MaxMemory"`
	EvictionPolicy     string        `json:"EvictionPolicy" yaml:"EvictionPolicy"`
	EvictionSample     uint          `json:"EvictionSample" yaml:"EvictionSample"`
	EvictionInterval   time.Duration `json:"EvictionInterval" yaml:"EvictionInterval"`
}

func GetConfig() (Config, error) {
	var certKeyPairs [][]string
	var clientCAs []string

	flag.Func("cert-key-pair",
		"A pair of file paths representing the signed certificate and it's corresponding key separated by a comma.",
		func(s string) error {
			pair := strings.Split(strings.TrimSpace(s), ",")
			for i := 0; i < len(pair); i++ {
				pair[i] = strings.TrimSpace(pair[i])
			}
			if len(pair) != 2 {
				return errors.New("certKeyPair must be 2 comma separated strings")
			}
			certKeyPairs = append(certKeyPairs, pair)
			return nil
		})

	flag.Func("client-ca", "Path to certificate authority used to verify client certificates.", func(s string) error {
		clientCAs = append(clientCAs, s)
		return nil
	})

	aofSyncStrategy := "everysec"
	flag.Func("aof-sync-strategy", `How often to flush the file contents written to append only file.
The options are 'always' for syncing on each command, 'everysec' to sync every second, and 'no' to leave it up to the os.`,
		func(option string) error {
			if !slices.ContainsFunc([]string{"always", "everysec", "no"}, func(s string) bool {
				return strings.EqualFold(s, option)
			}) {
				return errors.New("aofSyncStrategy must be 'always', 'everysec' or 'no'")
			}
			aofSyncStrategy = strings.ToLower(option)
			return nil
		})

	var maxMemory uint64 = 0
	flag.Func("max-memory", `Upper memory limit before triggering eviction. 
Supported units (kb, mb, gb, tb, pb). When 0 is passed, there will be no memory limit.
There is no limit by default.`, func(memory string) error {
		b, err := internal.ParseMemory(memory)
		if err != nil {
			return err
		}
		maxMemory = b
		return nil
	})

	evictionPolicy := constants.NoEviction
	flag.Func("eviction-policy",
		`The eviction policy used to remove keys when max-memory is reached. The options are: 
1) noeviction - Do not evict any keys even when max-memory is exceeded.
2) allkeys-lfu - Evict the least frequently used keys.
3) allkeys-lru - Evict the least recently used keys.
4) volatile-lfu - Evict the least frequently used keys with an expiration.
5) volatile-lru - Evict the least recently used keys with an expiration.
6) allkeys-random - Evict random keys until we get under the max-memory limit.
7) volatile-random - Evict random keys with an expiration.`, func(policy string) error {
			policies := []string{
				constants.NoEviction,
				constants.AllKeysLFU, constants.AllKeysLRU, constants.AllKeysRandom,
				constants.VolatileLFU, constants.VolatileLRU, constants.VolatileRandom,
			}
			policyIdx := slices.Index(policies, strings.ToLower(policy))
			if policyIdx == -1 {
				return fmt.Errorf("policy %s is not a valid policy", policy)
			}
			evictionPolicy = strings.ToLower(policy)
			return nil
		})

	tls := flag.Bool("tls", false, "Start the echovault in TLS mode. Default is false.")
	mtls := flag.Bool("mtls", false, "Use mTLS to verify the client.")
	port := flag.Int("port", 7480, "Port to use. Default is 7480")
	serverId := flag.String("server-id", "1", "EchoVault ID in raft cluster. Leave empty for client.")
	joinAddr := flag.String("join-addr", "", "Address of cluster member in a cluster to you want to join.")
	bindAddr := flag.String("bind-addr", "", "Address to bind the echovault to.")
	raftBindPort := flag.Uint("raft-port", 7481, "Port to use for intra-cluster communication. Leave on the client.")
	mlBindPort := flag.Uint("memberlist-port", 7946, "Port to use for memberlist communication.")
	inMemory := flag.Bool("in-memory", false, "Whether to use memory or persistent storage for raft logs and snapshots.")
	dataDir := flag.String("data-dir", "/var/lib/echovault", "Directory to store snapshots and logs.")
	bootstrapCluster := flag.Bool("bootstrap-cluster", false, "Whether this instance should bootstrap a new cluster.")
	aclConfig := flag.String("acl-config", "", "ACL config file path.")
	snapshotThreshold := flag.Uint64("snapshot-threshold", 1000, "The number of entries that trigger a snapshot. Default is 1000.")
	snapshotInterval := flag.Duration("snapshot-interval", 5*time.Minute, "The time interval between snapshots (in seconds). Default is 5 minutes.")
	restoreSnapshot := flag.Bool("restore-snapshot", false, "This flag prompts the echovault to restore state from snapshot when set to true. Only works in standalone mode. Higher priority than restoreAOF.")
	restoreAOF := flag.Bool("restore-aof", false, "This flag prompts the echovault to restore state from append-only logs. Only works in standalone mode. Lower priority than restoreSnapshot.")
	evictionSample := flag.Uint("eviction-sample", 20, "An integer specifying the number of keys to sample when checking for expired keys.")
	evictionInterval := flag.Duration("eviction-interval", 100*time.Millisecond, "The interval between each sampling of keys to evict.")
	forwardCommand := flag.Bool(
		"forward-commands",
		false,
		"If the node is a follower, this flag forwards mutation command to the leader when set to true")
	requirePass := flag.Bool(
		"require-pass",
		false,
		"Whether the echovault should require a password before allowing commands. Default is false.",
	)
	password := flag.String(
		"password",
		"",
		`The password for the default user. ACL config file will overwrite this value. 
It is a plain text value by default but you can provide a SHA256 hash by adding a '#' before the hash.`,
	)

	config := flag.String(
		"config",
		"",
		`File path to a JSON or YAML config file.The values in this config file will override the flag values.`,
	)

	flag.Parse()

	conf := Config{
		CertKeyPairs:       certKeyPairs,
		ClientCAs:          clientCAs,
		TLS:                *tls,
		MTLS:               *mtls,
		Port:               uint16(*port),
		ServerID:           *serverId,
		JoinAddr:           *joinAddr,
		BindAddr:           *bindAddr,
		RaftBindPort:       uint16(*raftBindPort),
		MemberListBindPort: uint16(*mlBindPort),
		InMemory:           *inMemory,
		DataDir:            *dataDir,
		BootstrapCluster:   *bootstrapCluster,
		AclConfig:          *aclConfig,
		ForwardCommand:     *forwardCommand,
		RequirePass:        *requirePass,
		Password:           *password,
		SnapShotThreshold:  *snapshotThreshold,
		SnapshotInterval:   *snapshotInterval,
		RestoreSnapshot:    *restoreSnapshot,
		RestoreAOF:         *restoreAOF,
		AOFSyncStrategy:    aofSyncStrategy,
		MaxMemory:          maxMemory,
		EvictionPolicy:     evictionPolicy,
		EvictionSample:     *evictionSample,
		EvictionInterval:   *evictionInterval,
	}

	if len(*config) > 0 {
		// Override configurations from file
		if f, err := os.Open(*config); err != nil {
			panic(err)
		} else {
			defer func() {
				if err = f.Close(); err != nil {
					log.Println(err)
				}
			}()

			ext := path.Ext(f.Name())

			if ext == ".json" {
				if err = json.NewDecoder(f).Decode(&conf); err != nil {
					return Config{}, nil
				}
			}

			if ext == ".yaml" || ext == ".yml" {
				if err = yaml.NewDecoder(f).Decode(&conf); err != nil {
					return Config{}, err
				}
			}
		}
	}

	// If requirePass is set to true, then password must be provided as well
	var err error = nil

	if conf.RequirePass && conf.Password == "" {
		err = errors.New("password cannot be empty if requirePass is generic to true")
	}

	return conf, err
}
