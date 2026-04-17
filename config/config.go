package config

import "time"

type Config struct {
	DAG       DAGConfig
	Sync      SyncConfig
	Request   RequestConfig
	Consensus ConsensusConfig
	Resource  ResourceConfig
	Security  SecurityConfig
}

type DAGConfig struct {
	OrphanCapacity       int           `yaml:"orphan_capacity"`
	OrphanTTL            time.Duration `yaml:"orphan_ttl"`
	SyncTriggerThreshold int           `yaml:"sync_trigger_threshold"`
}

type SyncConfig struct {
	EnableStepSync bool          `yaml:"enable_step_sync"` // 임시 구조임을 명시!
	Step1Timeout   time.Duration `yaml:"step1_timeout"`
	Step2Timeout   time.Duration `yaml:"step2_timeout"`
	Step3Timeout   time.Duration `yaml:"step3_timeout"`
	MaxRandomPeers int           `yaml:"max_random_peers"`
}

type RequestConfig struct {
	BaseTimeout           time.Duration `yaml:"base_timeout"`
	MaxBackoff            time.Duration `yaml:"max_backoff"`
	MaxFetchRequestHashes int           `yaml:"max_fetch_request_hashes"`
	MaxFetchResponseVtx   int           `yaml:"max_fetch_response_vtx"`
}

type ConsensusConfig struct {
	// 정족수 계산을 위한 비율 (0.0 ~ 1.0)
	GlobalQuorumRatio    float64 `yaml:"global_quorum_ratio"`    // 0.67 (2/3)
	CommitteeQuorumRatio float64 `yaml:"committee_quorum_ratio"` // 0.75 (3/4)
}

type SecurityConfig struct {
	SlashThreshold      int `yaml:"slash_threshold"`
	EquivocationPenalty int `yaml:"equivocation_penalty"`
}

type ResourceConfig struct {
	FetcherChannelSize   int `yaml:"fetcher_channel_size"`
	ValidatorChannelSize int `yaml:"validator_channel_size"`
}


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
			BaseTimeout:           10 * time.Second,
			MaxBackoff:            640 * time.Second, // 10s * 2^6 (최대 6회 리트라이 가정)
			MaxFetchRequestHashes: 100,
			MaxFetchResponseVtx:   50,
		},
		Consensus: ConsensusConfig{
			GlobalQuorumRatio:    0.67,
			CommitteeQuorumRatio: 0.75,
		},
		Security: SecurityConfig{
			SlashThreshold:      100,
			EquivocationPenalty: 100,
		},
		Resource: ResourceConfig{
			FetcherChannelSize:   100,
			ValidatorChannelSize: 1024,
		},
	}
}
