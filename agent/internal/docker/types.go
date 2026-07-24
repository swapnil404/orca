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
	Config    *ConfigMount
}

// ConfigMount describes generated configuration persisted on the host and
// bind-mounted read-only into a container.
type ConfigMount struct {
	RelativePath  string
	ContainerPath string
	Content       string
}

// ContainerInfo describes an Orca container currently visible in Docker.
type ContainerInfo struct {
	ID        string
	Name      string
	ClusterID string
	Kind      ContainerKind
	ReplicaID string
	Image     string
	Status    string
	Config    string
}
