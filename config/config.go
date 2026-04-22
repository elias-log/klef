// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package config

import (
	"math"
	"time"
)

/// Config represents the root configuration structure for the Arachnet node.
type Config struct {
	NodeID  int    `yaml:"node_id"`
	Keypath string `yaml:"keypath"`

	DAG       DAGConfig
	Sync      SyncConfig
	Request   RequestConfig
	Consensus ConsensusConfig
	Resource  ResourceConfig
	Security  SecurityConfig
}

/// DAGConfig defines operational parameters for the Directed Acyclic Graph.
type DAGConfig struct {
	OrphanCapacity       int           `yaml:"orphan_capacity"`
	OrphanTTL            time.Duration `yaml:"orphan_ttl"`
	SyncTriggerThreshold int           `yaml:"sync_trigger_threshold"`
}

/// SyncConfig manages the tiered synchronization strategy.
type SyncConfig struct {
	// EnableStepSync: Note that the current step-based sync is a transitional architecture.
	EnableStepSync bool          `yaml:"enable_step_sync"`
	Step1Timeout   time.Duration `yaml:"step1_timeout"`    // Direct request timeout (Step 1).
	Step2Timeout   time.Duration `yaml:"step2_timeout"`    // Neighbor fan-out timeout (Step 2).
	Step3Timeout   time.Duration `yaml:"step3_timeout"`    // Network broadcast timeout (Step 3).
	MaxRandomPeers int           `yaml:"max_random_peers"` // Peer count for randomized fan-out.
}

/// RequestConfig sets limits and timeouts for individual p2p requests.
type RequestConfig struct {
	BaseTimeout           time.Duration `yaml:"base_timeout"`
	MaxBackoff            time.Duration `yaml:"max_backoff"`
	MaxFetchRequestHashes int           `yaml:"max_fetch_request_hashes"` // Max hashes per FETCH_REQ.
	MaxFetchResponseVtx   int           `yaml:"max_fetch_response_vtx"`   // Max vertices per FETCH_RES.
}

/// ConsensusConfig contains parameters for quorum calculations and node counts.
type ConsensusConfig struct {
	TotalNodes     int `yaml:"total_nodes"`     // Total nodes in the entire network (n).
	CommitteeNodes int `yaml:"committee_nodes"` // Total nodes within the specific consensus committee.

	// Ratios for quorum calculations (0.0 ~ 1.0).
	GlobalQuorumRatio    float64 `yaml:"global_quorum_ratio"`    // Typically 0.67 (2/3).
	CommitteeQuorumRatio float64 `yaml:"committee_quorum_ratio"` // Typically 0.75 (3/4).
}

/// SecurityConfig defines thresholds for the Slasher and penalty mechanisms.
type SecurityConfig struct {
	SlashThreshold         int `yaml:"slash_threshold"`          // Score at which a node is officially slashed.
	MalformedVertexPenalty int `yaml:"malformed_vertex_penalty"` // Penalty for sending invalid vertex data.
	EquivocationPenalty    int `yaml:"equivocation_penalty"`     // Penalty for double-voting/signing.
}

/// ResourceConfig allocates internal buffer and channel sizes.
type ResourceConfig struct {
	FetcherChannelSize   int `yaml:"fetcher_channel_size"`
	ValidatorChannelSize int `yaml:"validator_channel_size"`
}

/// DefaultConfig initializes the node with recommended baseline parameters.
func DefaultConfig() *Config {
	return &Config{
		DAG: DAGConfig{
			OrphanCapacity:       10000,
			OrphanTTL:            30 * time.Minute,
			SyncTriggerThreshold: 2,
		},
		Sync: SyncConfig{
			EnableStepSync: true,
			Step1Timeout:   300 * time.Millisecond,
			Step2Timeout:   500 * time.Millisecond,
			Step3Timeout:   1000 * time.Millisecond,
			MaxRandomPeers: 3,
		},
		Request: RequestConfig{
			BaseTimeout:           500 * time.Millisecond,
			MaxBackoff:            640 * time.Second,
			MaxFetchRequestHashes: 100,
			MaxFetchResponseVtx:   50,
		},
		Consensus: ConsensusConfig{
			TotalNodes:           4, // Minimum for 3f+1 resilience.
			CommitteeNodes:       4,
			GlobalQuorumRatio:    0.67,
			CommitteeQuorumRatio: 0.75,
		},
		Security: SecurityConfig{
			SlashThreshold:         100,
			MalformedVertexPenalty: 15,
			EquivocationPenalty:    100,
		},
		Resource: ResourceConfig{
			FetcherChannelSize:   100,
			ValidatorChannelSize: 1024,
		},
	}
}

/// LoadConfig prepares the node configuration, overwriting defaults with environment or file data.
func LoadConfig() *Config {
	cfg := DefaultConfig()

	// TODO: Integrate YAML unmarshaling logic here.
	cfg.NodeID = 0
	cfg.Keypath = "keys/node0.key"

	return cfg
}

/// GetGlobalThreshold calculates the required quorum count for the entire network (> 2/3).
func (c *Config) GetGlobalThreshold(totalNodes int) int {
	return int(math.Floor(float64(totalNodes)*c.Consensus.GlobalQuorumRatio)) + 1
}

/// GetCommitteeThreshold calculates the required quorum count for a committee (> 3/4).
func (c *Config) GetCommitteeThreshold(committeeNodes int) int {
	return int(math.Floor(float64(committeeNodes)*c.Consensus.CommitteeQuorumRatio)) + 1
}
