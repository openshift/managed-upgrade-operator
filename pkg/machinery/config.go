package machinery

import (
	"time"
)

type NodeDrain struct {
	Timeout int `yaml:"timeOut" default:"45"`
}

func (nd *NodeDrain) GetDuration() time.Duration {
	return time.Duration(nd.Timeout) * time.Minute
}
