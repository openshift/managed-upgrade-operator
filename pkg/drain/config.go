package drain

import (
	"time"
)

// NodeDrain holds timeout and expected drain time fields required for NodeDrain execution
type NodeDrain struct {
	Timeout               int `yaml:"timeOut"`
	ExpectedNodeDrainTime int `yaml:"expectedNodeDrainTime" default:"8"`
}

// GetTimeOutDuration returns the timout field from the NodeDrain object
func (nd *NodeDrain) GetTimeOutDuration() time.Duration {
	return time.Duration(nd.Timeout) * time.Minute
}

// GetExpectedDrainDuration returns the ExpectedNodeDrainTime field from the NodeDrain object
func (nd *NodeDrain) GetExpectedDrainDuration() time.Duration {
	return time.Duration(nd.ExpectedNodeDrainTime) * time.Minute
}
