package models

type System struct {
	CPUId         string   `json:"cpu_idle" bson:"cpu_idle"`
	Hostname      string   `json:"hostname" bson:"hostname"`
	Version       string   `json:"version" bson:"version"`
	Release       string   `json:"release" bson:"release"`
	BootTime      uint64   `json:"boot_time" bson:"boot_time"`
	KernelVersion string   `json:"kernel_version" bson:"kernel_version"`
	MACs          []string `json:"macs" bson:"macs"`
	IPs           []string `json:"ips" bson:"ips"`
	Architecture  string   `json:"architecture" bson:"architecture"`
	UsedMemory    uint64   `json:"used_memory" bson:"used_memory"`
	TotalMemory   uint64   `json:"total_memory" bson:"total_memory"`
	FreeMemory    uint64   `json:"free_memory" bson:"free_memory"`
}
