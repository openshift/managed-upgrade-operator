package ocm

// UpgradePolicyList represents an unmarshalled Upgrade Policy response from Cluster Services
type UpgradePolicyList struct {
	Kind  string          `json:"kind"`
	Page  int64           `json:"page"`
	Size  int64           `json:"size"`
	Total int64           `json:"total"`
	Items []UpgradePolicy `json:"items"`
}

// UpgradePolicy represents an unmarshalled individual Upgrade Policy response from Cluster Services
type UpgradePolicy struct {
	Id                  string `json:"id"`
	Kind                string `json:"kind"`
	Href                string `json:"href"`
	Schedule            string `json:"schedule"`
	ScheduleType        string `json:"schedule_type"`
	UpgradeType         string `json:"upgrade_type"`
	Version             string `json:"version"`
	NextRun             string `json:"next_run"`
	PrevRun             string `json:"prev_run"`
	ClusterId           string `json:"cluster_id"`
	CapacityReservation *bool  `json:"capacity_reservation"`
}

// ClusterList represents an unmarshalled Cluster List response from Cluster Services
type ClusterList struct {
	Kind  string        `json:"kind"`
	Page  int64         `json:"page"`
	Size  int64         `json:"size"`
	Total int64         `json:"total"`
	Items []ClusterInfo `json:"items"`
}

// ClusterInfo represents a partial unmarshalled Cluster response from Cluster Services
type ClusterInfo struct {
	Id                   string               `json:"id"`
	Version              ClusterVersion       `json:"version"`
	NodeDrainGracePeriod NodeDrainGracePeriod `json:"node_drain_grace_period"`
}

// NodeDrainGracePeriod represents a duration for node drain grace periods
type NodeDrainGracePeriod struct {
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
}

// ClusterVersion represents a clusters version
type ClusterVersion struct {
	Id           string `json:"id"`
	ChannelGroup string `json:"channel_group"`
}

// UpgradePolicyStateRequest represents an Upgrade Policy state for notifications
type UpgradePolicyStateRequest struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

// UpgradePolicyState represents an Upgrade Policy state for notifications
type UpgradePolicyState struct {
	Kind        string `json:"kind"`
	Href        string `json:"href"`
	Value       string `json:"value"`
	Description string `json:"description"`
}
