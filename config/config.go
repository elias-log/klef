package config

import "time"

type Config struct {
	// DAG 관련 설정
	OrphanCapacity       int           `yaml:"orphan_capacity"`
	OrphanTTL            time.Duration `yaml:"orphan_ttl"`
	SyncTriggerThreshold int           `yaml:"sync_trigger_threshold"`

	// Fetcher 관련 설정
	MaxRandomPeers        int `yaml:"max_random_peers"`
	MaxFetchRequestHashes int `yaml:"max_fetch_request_hashes"`
	MaxFetchResponseVtx   int `yaml:"max_fetch_response_vtx"`

	//Timeout
	SyncStep1Timeout time.Duration `yaml:"sync_step1_timeout"`
	SyncStep2Timeout time.Duration `yaml:"sync_step2_timeout"`
	SyncStep3Timeout time.Duration `yaml:"sync_step3_timeout"`

	// Validator 관련 설정
	RequestTimeout     time.Duration `yaml:"request_timeout"`
	FetcherChannelSize int           `yaml:"fetcher_channel_size"`

	// Slasher 관련 설정 (미래에 CoreConfig로 분리될 후보들일세)
	SlashThreshold      int `yaml:"slash_threshold"`      // 이 점수를 넘으면 퇴출!
	EquivocationPenalty int `yaml:"equivocation_penalty"` // 이중 투표 시 벌점

	// Global Quorum (2f+1)
	GlobalQuorumNumerator   int // 2
	GlobalQuorumDenominator int // 3

	// Committee Quorum (3/4)
	CommitteeQuorumNumerator   int // 3
	CommitteeQuorumDenominator int // 4

}

// DefaultConfig
func DefaultConfig() *Config {
	return &Config{
		OrphanCapacity:             10000,
		OrphanTTL:                  30 * time.Minute,
		SyncTriggerThreshold:       2,
		MaxRandomPeers:             3,
		MaxFetchRequestHashes:      100,
		MaxFetchResponseVtx:        50,
		SyncStep1Timeout:           300 * time.Millisecond,
		SyncStep2Timeout:           500 * time.Millisecond,
		SyncStep3Timeout:           1000 * time.Millisecond,
		RequestTimeout:             10 * time.Second,
		FetcherChannelSize:         100,
		SlashThreshold:             100,
		EquivocationPenalty:        100, // 한 번만 걸려도 바로 아웃이구먼!
		GlobalQuorumNumerator:      2,
		GlobalQuorumDenominator:    3,
		CommitteeQuorumNumerator:   3,
		CommitteeQuorumDenominator: 4,
	}
}
