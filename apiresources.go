package executor

import (
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/tedsuo/ifrit"
)

const (
	StateReserved     = "reserved"
	StateInitializing = "initializing"
	StateCreated      = "created"
	StateCompleted    = "completed"
)

type Container struct {
	Guid string `json:"guid"`

	// alloc
	MemoryMB int `json:"memory_mb"`
	DiskMB   int `json:"disk_mb"`

	Tags Tags `json:"tags,omitempty"`

	AllocatedAt int64 `json:"allocated_at"`

	// init
	RootFSPath string        `json:"root_fs"`
	CPUWeight  uint          `json:"cpu_weight"`
	Ports      []PortMapping `json:"ports"`
	Log        LogConfig     `json:"log"`

	// run
	Actions     []models.ExecutorAction `json:"actions"`
	Env         []EnvironmentVariable   `json:"env,omitempty"`
	CompleteURL string                  `json:"complete_url"`

	RunResult ContainerRunResult `json:"run_result"`

	// internally updated
	State           string        `json:"state"`
	ContainerHandle string        `json:"container_handle"`
	Process         ifrit.Process `json:"-"`
}

type EnvironmentVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type LogConfig struct {
	Guid       string `json:"guid"`
	SourceName string `json:"source_name"`
	Index      *int   `json:"index"`
}

type PortMapping struct {
	ContainerPort uint32 `json:"container_port"`
	HostPort      uint32 `json:"host_port,omitempty"`
}

type ContainerRunResult struct {
	Guid string `json:"guid"`

	Failed        bool   `json:"failed"`
	FailureReason string `json:"failure_reason"`
}

type ExecutorResources struct {
	MemoryMB   int `json:"memory_mb"`
	DiskMB     int `json:"disk_mb"`
	Containers int `json:"containers"`
}

type Tags map[string]string
