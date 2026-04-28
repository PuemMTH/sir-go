package main

type ServiceStatus int

const (
	StatusRunning ServiceStatus = iota
	StatusStopped
	StatusOther
	StatusError
)

type containerInfo struct {
	ID      string // truncated 12-char display ID
	FullID  string // full container ID for docker operations
	State   string
	Created int64
	Image   string
	Ports   string
}

type Row struct {
	Num             int
	Folder          string
	Compose         string
	Service         string
	State           string
	Uptime          string
	ContainerID     string
	FullContainerID string // full ID for exec/stop/restart
	Image           string
	Ports           string
	Status          ServiceStatus
}

type ScanConfig struct {
	Depth     int
	FullPath  bool
	Technical bool
}

type composeDir struct {
	dir          string
	relFolder    string
	composeFiles []string
}

type scanResult struct {
	rows             []Row
	total, run, stop int
}
