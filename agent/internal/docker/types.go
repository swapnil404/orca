package docker

import "context"

// DockerClient manages Orca containers and volumes through Docker.
type DockerClient interface {
	CreateContainer(ctx context.Context, spec ContainerSpec) (containerID string, err error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string) error
	RemoveContainer(ctx context.Context, containerID string) error
	EnsureVolume(ctx context.Context, name string) error
	RemoveVolume(ctx context.Context, name string) error
	RemoveNetwork(ctx context.Context, name string) error
	RemoveClusterData(ctx context.Context, clusterID string) error
	ListOrcaContainers(ctx context.Context) ([]ContainerInfo, error)
}

// ContainerKind identifies the Orca role a container serves.
type ContainerKind string

const (
	// ContainerKindPrimary is a Postgres primary container.
	ContainerKindPrimary ContainerKind = "primary"
	// ContainerKindReplica is a Postgres replica container.
	ContainerKindReplica ContainerKind = "replica"
	// ContainerKindPgBouncer is a PgBouncer container.
	ContainerKindPgBouncer ContainerKind = "pgbouncer"
	// ContainerKindPgBackRest is a temporary pgBackRest restore container.
	ContainerKindPgBackRest ContainerKind = "pgbackrest"
)

// ContainerSpec describes the container the Docker wrapper should create.
type ContainerSpec struct {
	ClusterID string
	Kind      ContainerKind
	ReplicaID string
	Image     string
	Env       []string
	Labels    map[string]string
	Command   []string
	UseVolume bool
	Volumes   []VolumeMount
	Binds     []BindMount
	Config    *ConfigMount
	Configs   []*ConfigMount
	Ports     []PublishedPort
}

// PublishedPort describes a container port published on the agent host.
type PublishedPort struct {
	ContainerPort uint16
	HostAddress   string
	HostPort      uint16
}

// VolumeMount describes an explicit named-volume mount.
type VolumeMount struct {
	Name     string
	Path     string
	ReadOnly bool
}

// BindMount describes a host path mounted into a container.
type BindMount struct {
	Source   string
	Path     string
	ReadOnly bool
	Create   bool
}

// ConfigMount describes generated configuration persisted on the host and
// bind-mounted read-only into a container.
type ConfigMount struct {
	RelativePath  string
	ContainerPath string
	Content       string
	Mode          uint32
}

// ContainerInfo describes an Orca container currently visible in Docker.
type ContainerInfo struct {
	ID                string
	Name              string
	ClusterID         string
	Kind              ContainerKind
	ReplicaID         string
	Image             string
	Status            string
	Config            string
	BackupConfig      string
	RestartGeneration uint64
	NetworkName       string
	PublishedAddress  string
	PublishedPort     uint16
}

// VolumeInfo describes an Orca data volume visible in Docker.
type VolumeInfo struct {
	Name      string
	ClusterID string
}
