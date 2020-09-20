package drain

import (
	"time"
)

type NodeDrain struct {
	Timeout               int `yaml:"timeOut"`
	ExpectedNodeDrainTime int `yaml:"expectedNodeDrainTime" default:"8"`
}

func (nd *NodeDrain) GetTimeOutDuration() time.Duration {
	return time.Duration(nd.Timeout) * time.Minute
}

func (nd *NodeDrain) GetExpectedDrainDuration() time.Duration {
	return time.Duration(nd.ExpectedNodeDrainTime) * time.Minute
}
