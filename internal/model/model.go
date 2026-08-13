package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

var (
	ErrInvalidInstanceName = errors.New("instance name must be a lowercase machine label of 1 to 63 letters, digits, or hyphens")
	instanceNamePattern    = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

type Instance struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	CreatedAt         time.Time       `json:"created_at"`
	LastSeenAt        *time.Time      `json:"last_seen_at,omitempty"`
	AgentVersion      string          `json:"agent_version,omitempty"`
	Capabilities      []string        `json:"capabilities"`
	DesiredGeneration uint64          `json:"desired_generation"`
	DesiredDigest     string          `json:"desired_digest,omitempty"`
	DesiredSpec       json.RawMessage `json:"desired_spec,omitempty"`
	AppliedGeneration uint64          `json:"applied_generation"`
	AppliedDigest     string          `json:"applied_digest,omitempty"`
	Phase             string          `json:"phase"`
	Reason            string          `json:"reason,omitempty"`
}

func ValidateInstanceName(name string) error {
	if !instanceNamePattern.MatchString(name) {
		return ErrInvalidInstanceName
	}
	return nil
}

type InstanceSpec struct {
	Engine       string            `json:"engine"`
	Artifact     string            `json:"artifact"`
	DesiredPhase string            `json:"desired_phase"`
	Config       json.RawMessage   `json:"config"`
	Labels       map[string]string `json:"labels,omitempty"`
}

func CanonicalSpec(spec InstanceSpec) ([]byte, string, error) {
	canonical, err := json.Marshal(spec)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:]), nil
}

type Event struct {
	ID         uint64         `json:"id"`
	Type       string         `json:"type"`
	ResourceID string         `json:"resource_id,omitempty"`
	At         time.Time      `json:"at"`
	Data       map[string]any `json:"data,omitempty"`
}

type Usage struct {
	UplinkBytes   uint64 `json:"uplink_bytes"`
	DownlinkBytes uint64 `json:"downlink_bytes"`
}

type Dashboard struct {
	Instances struct {
		Total   int `json:"total"`
		Online  int `json:"online"`
		Running int `json:"running"`
		Failed  int `json:"failed"`
	} `json:"instances"`
	Traffic Usage     `json:"traffic"`
	Now     time.Time `json:"now"`
}
