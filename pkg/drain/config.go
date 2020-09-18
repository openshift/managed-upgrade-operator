package drain

import (
	"time"
)

type NodeDrain struct {
	Timeout        int `yaml:"timeOut"`
	WorkerNodeTime int `yaml:"workerNodeTime" default:"8"`
}

func (nd *NodeDrain) GetTimeOutDuration() time.Duration {
	return time.Duration(nd.Timeout) * time.Minute
}

func (nd *NodeDrain) GetExpectedDrainDuration() time.Duration {
	return time.Duration(nd.WorkerNodeTime) * time.Minute
}
