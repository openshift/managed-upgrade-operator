package machinery

import (
	"time"
)

type NodeDrain struct {
	Timeout int `yaml:"timeOut"`
}

func (nd *NodeDrain) GetDuration() time.Duration {
	return time.Duration(nd.Timeout) * time.Minute
}
