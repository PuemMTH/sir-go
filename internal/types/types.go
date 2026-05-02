package types

type ServiceStatus int

const (
	StatusRunning ServiceStatus = iota
	StatusStopped
	StatusOther
	StatusError
)

type Row struct {
	Num             int
	Folder          string
	Compose         string
	Service         string
	State           string
	Uptime          string
	ContainerID     string
	FullContainerID string
	Image           string
	Ports           string
	Status          ServiceStatus
}

type ScanConfig struct {
	Depth     int
	FullPath  bool
	Technical bool
}

type ScanResult struct {
	Rows             []Row
	Total, Run, Stop int
}
