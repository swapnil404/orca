package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dockertypes "github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/stdcopy"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	postgresimage "github.com/swapnil404/orca/agent/images/postgres"
)

const (
	orcaNamePrefix = "orca-"
	dataRoot       = "/var/orca/data"
	// PgBouncerConfigRelativePath is the generated INI path within a cluster's data directory.
	PgBouncerConfigRelativePath = "pgbouncer/pgbouncer.ini"
	// PgBouncerConfigContainerPath is where the generated INI is mounted in PgBouncer.
	PgBouncerConfigContainerPath = "/etc/pgbouncer/pgbouncer.ini"
	// PostgresConfigRelativePath is the generated PostgreSQL include path.
	PostgresConfigRelativePath = "postgres/postgresql.conf"
	// PostgresConfigContainerPath is where the generated include is mounted.
	PostgresConfigContainerPath = "/etc/orca/postgresql.conf"
	// PostgresAppliedConfigRelativePath records the last successfully applied parameters.
	PostgresAppliedConfigRelativePath = "postgres/applied.conf"
	// PgBackRestAppliedConfigRelativePath records the last successfully applied backup configuration.
	PgBackRestAppliedConfigRelativePath = "pgbackrest/applied.conf"
)

type sdkClient interface {
	ContainerCreate(ctx context.Context, config *containertypes.Config, hostConfig *containertypes.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (containertypes.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options containertypes.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options containertypes.StopOptions) error
	ContainerRemove(ctx context.Context, containerID string, options containertypes.RemoveOptions) error
	ContainerList(ctx context.Context, options containertypes.ListOptions) ([]dockertypes.Container, error)
	ImageBuild(ctx context.Context, buildContext io.Reader, options dockertypes.ImageBuildOptions) (dockertypes.ImageBuildResponse, error)
	ImagePull(ctx context.Context, ref string, options imagetypes.PullOptions) (io.ReadCloser, error)
	VolumeCreate(ctx context.Context, options volumetypes.CreateOptions) (volumetypes.Volume, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
}

type execSDKClient interface {
	ContainerExecCreate(ctx context.Context, container string, options containertypes.ExecOptions) (dockertypes.IDResponse, error)
	ContainerExecAttach(ctx context.Context, execID string, config containertypes.ExecAttachOptions) (dockertypes.HijackedResponse, error)
	ContainerExecInspect(ctx context.Context, execID string) (containertypes.ExecInspect, error)
}

type inspectSDKClient interface {
	ContainerInspect(ctx context.Context, containerID string) (dockertypes.ContainerJSON, error)
}

type restartSDKClient interface {
	ContainerRestart(ctx context.Context, containerID string, options containertypes.StopOptions) error
}

type volumeListSDKClient interface {
	VolumeList(ctx context.Context, options volumetypes.ListOptions) (volumetypes.ListResponse, error)
}

// Client implements DockerClient using the official Docker SDK for Go.
type Client struct {
	sdk      sdkClient
	dataRoot string
}

// NewClient creates a Docker SDK-backed Client from environment configuration.
func NewClient() (*Client, error) {
	sdk, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	dockerClient := NewClientWithSDK(sdk)
	if configuredRoot := os.Getenv("ORCA_DATA_DIR"); configuredRoot != "" {
		if !filepath.IsAbs(configuredRoot) {
			return nil, errors.New("ORCA_DATA_DIR must be an absolute path")
		}
		dockerClient.dataRoot = filepath.Clean(configuredRoot)
	}
	return dockerClient, nil
}

// NewClientWithSDK creates a Client using the provided Docker SDK-compatible client.
func NewClientWithSDK(sdk sdkClient) *Client {
	return &Client{sdk: sdk, dataRoot: dataRoot}
}

// CreateContainer creates an Orca container using the exact Orca naming convention.
func (c *Client) CreateContainer(ctx context.Context, spec ContainerSpec) (string, error) {
	if c.sdk == nil {
		return "", errors.New("docker client is nil")
	}

	name, err := ContainerName(spec)
	if err != nil {
		return "", err
	}
	if spec.Image == "" {
		return "", errors.New("container image is required")
	}

	labels := make(map[string]string, len(spec.Labels)+3)
	for key, value := range spec.Labels {
		labels[key] = value
	}
	labels["orca.cluster_id"] = spec.ClusterID
	labels["orca.kind"] = string(spec.Kind)
	if spec.ReplicaID != "" {
		labels["orca.replica_id"] = spec.ReplicaID
	}

	hostConfig := &containertypes.HostConfig{}
	if spec.UseVolume {
		volumeName := VolumeName(spec.ClusterID)
		if err := c.EnsureVolume(ctx, volumeName); err != nil {
			return "", err
		}

		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: volumeName,
			Target: VolumeMountPath(spec.ClusterID),
		})
	}
	if spec.Config != nil {
		configMount, err := writeConfigMount(c.dataRoot, spec.ClusterID, spec.Config)
		if err != nil {
			return "", err
		}
		hostConfig.Mounts = append(hostConfig.Mounts, configMount)
	}

	config := &containertypes.Config{
		Image:  spec.Image,
		Env:    spec.Env,
		Cmd:    spec.Command,
		Labels: labels,
	}
	created, err := c.sdk.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if errdefs.IsNotFound(err) {
		if err := c.ensureMissingImage(ctx, spec.Image); err != nil {
			return "", err
		}
		created, err = c.sdk.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	}
	if err != nil {
		return "", err
	}

	return created.ID, nil
}

func (c *Client) ensureMissingImage(ctx context.Context, image string) error {
	if strings.HasPrefix(image, "orca-postgres:") {
		return c.buildOrcaPostgresImage(ctx, image)
	}
	return c.pullImage(ctx, image)
}

func (c *Client) buildOrcaPostgresImage(ctx context.Context, image string) error {
	version := strings.TrimPrefix(image, "orca-postgres:")
	if version == "" || strings.ContainsAny(version, "/:@") {
		return fmt.Errorf("build pgBackRest-enabled PostgreSQL image %q: invalid PostgreSQL version tag", image)
	}

	var contextBuffer bytes.Buffer
	tarWriter := tar.NewWriter(&contextBuffer)
	dockerfile := postgresimage.Dockerfile()
	if err := tarWriter.WriteHeader(&tar.Header{Name: "Dockerfile", Mode: 0o644, Size: int64(len(dockerfile))}); err != nil {
		return fmt.Errorf("prepare build context for %q: %w", image, err)
	}
	if _, err := tarWriter.Write(dockerfile); err != nil {
		return fmt.Errorf("prepare build context for %q: %w", image, err)
	}
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("prepare build context for %q: %w", image, err)
	}

	response, err := c.sdk.ImageBuild(ctx, &contextBuffer, dockertypes.ImageBuildOptions{
		Tags:       []string{image},
		PullParent: true,
		Remove:     true,
		BuildArgs:  map[string]*string{"POSTGRES_VERSION": &version},
	})
	if err != nil {
		return fmt.Errorf("build pgBackRest-enabled PostgreSQL image %q: %w", image, err)
	}
	if response.Body == nil {
		return fmt.Errorf("build pgBackRest-enabled PostgreSQL image %q: Docker returned an empty response", image)
	}
	defer response.Body.Close()

	decoder := json.NewDecoder(response.Body)
	for {
		var message jsonmessage.JSONMessage
		if err := decoder.Decode(&message); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return fmt.Errorf("read build response for pgBackRest-enabled PostgreSQL image %q: %w", image, err)
		}
		if message.Error != nil {
			return fmt.Errorf("build pgBackRest-enabled PostgreSQL image %q: %s", image, message.Error.Message)
		}
		if message.ErrorMessage != "" {
			return fmt.Errorf("build pgBackRest-enabled PostgreSQL image %q: %s", image, message.ErrorMessage)
		}
	}

	return nil
}

// WriteConfig writes generated container configuration to the host.
func (c *Client) WriteConfig(ctx context.Context, clusterID string, config *ConfigMount) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeConfig(c.dataRoot, clusterID, config)
}

func (c *Client) pullImage(ctx context.Context, image string) error {
	stream, err := c.sdk.ImagePull(ctx, image, imagetypes.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %q: %w", image, err)
	}
	defer stream.Close()

	if _, err := io.Copy(io.Discard, stream); err != nil {
		return fmt.Errorf("read image pull response for %q: %w", image, err)
	}

	return nil
}

// StartContainer starts a Docker container by ID.
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	if c.sdk == nil {
		return errors.New("docker client is nil")
	}

	return c.sdk.ContainerStart(ctx, containerID, containertypes.StartOptions{})
}

// StopContainer stops a Docker container by ID.
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	if c.sdk == nil {
		return errors.New("docker client is nil")
	}

	return c.sdk.ContainerStop(ctx, containerID, containertypes.StopOptions{})
}

// RestartContainer restarts a Docker container by ID or name.
func (c *Client) RestartContainer(ctx context.Context, containerID string) error {
	if c.sdk == nil {
		return errors.New("docker client is nil")
	}
	sdk, ok := c.sdk.(restartSDKClient)
	if !ok {
		return errors.New("docker client does not support container restart")
	}

	return sdk.ContainerRestart(ctx, containerID, containertypes.StopOptions{})
}

// ExecContainer runs a command in a container and returns its standard output.
func (c *Client) ExecContainer(ctx context.Context, containerID string, command []string) (string, error) {
	if c.sdk == nil {
		return "", errors.New("docker client is nil")
	}
	if containerID == "" {
		return "", errors.New("container ID is required")
	}
	if len(command) == 0 {
		return "", errors.New("exec command is required")
	}
	sdk, ok := c.sdk.(execSDKClient)
	if !ok {
		return "", errors.New("docker client does not support container exec")
	}

	created, err := sdk.ContainerExecCreate(ctx, containerID, containertypes.ExecOptions{
		Cmd:          command,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("create exec in container %q: %w", containerID, err)
	}
	attached, err := sdk.ContainerExecAttach(ctx, created.ID, containertypes.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("attach exec in container %q: %w", containerID, err)
	}
	defer attached.Close()

	var stdout, stderr strings.Builder
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attached.Reader); err != nil {
		return "", fmt.Errorf("read exec output in container %q: %w", containerID, err)
	}
	result, err := sdk.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return "", fmt.Errorf("inspect exec in container %q: %w", containerID, err)
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("exec in container %q exited with code %d: %s", containerID, result.ExitCode, message)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// ContainerNetworkCIDRs returns the subnets attached to a container.
func (c *Client) ContainerNetworkCIDRs(ctx context.Context, containerID string) ([]string, error) {
	if c.sdk == nil {
		return nil, errors.New("docker client is nil")
	}
	if containerID == "" {
		return nil, errors.New("container ID is required")
	}
	sdk, ok := c.sdk.(inspectSDKClient)
	if !ok {
		return nil, errors.New("docker client does not support container inspect")
	}

	container, err := sdk.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect container %q: %w", containerID, err)
	}
	if container.NetworkSettings == nil {
		return nil, nil
	}

	cidrs := make(map[string]struct{}, len(container.NetworkSettings.Networks))
	for _, endpoint := range container.NetworkSettings.Networks {
		if endpoint == nil || endpoint.IPAddress == "" || endpoint.IPPrefixLen == 0 {
			continue
		}
		ip := net.ParseIP(endpoint.IPAddress)
		if ip == nil {
			continue
		}
		bits := 128
		if ip.To4() != nil {
			ip = ip.To4()
			bits = 32
		}
		network := &net.IPNet{IP: ip.Mask(net.CIDRMask(endpoint.IPPrefixLen, bits)), Mask: net.CIDRMask(endpoint.IPPrefixLen, bits)}
		cidrs[network.String()] = struct{}{}
	}

	result := make([]string, 0, len(cidrs))
	for cidr := range cidrs {
		result = append(result, cidr)
	}
	sort.Strings(result)
	return result, nil
}

// ContainerNetworkAddresses returns the IP addresses attached to a container.
func (c *Client) ContainerNetworkAddresses(ctx context.Context, containerID string) ([]string, error) {
	if c.sdk == nil {
		return nil, errors.New("docker client is nil")
	}
	if containerID == "" {
		return nil, errors.New("container ID is required")
	}
	sdk, ok := c.sdk.(inspectSDKClient)
	if !ok {
		return nil, errors.New("docker client does not support container inspect")
	}

	container, err := sdk.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect container %q: %w", containerID, err)
	}
	if container.NetworkSettings == nil {
		return nil, nil
	}

	addresses := make([]string, 0, len(container.NetworkSettings.Networks))
	for _, endpoint := range container.NetworkSettings.Networks {
		if endpoint != nil && net.ParseIP(endpoint.IPAddress) != nil {
			addresses = append(addresses, endpoint.IPAddress)
		}
	}
	sort.Strings(addresses)
	return addresses, nil
}

// RemoveContainer removes a Docker container by ID.
func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	if c.sdk == nil {
		return errors.New("docker client is nil")
	}

	return c.sdk.ContainerRemove(ctx, containerID, containertypes.RemoveOptions{})
}

// EnsureVolume creates the named Docker volume if it does not already exist.
func (c *Client) EnsureVolume(ctx context.Context, name string) error {
	if c.sdk == nil {
		return errors.New("docker client is nil")
	}
	if name == "" {
		return errors.New("volume name is required")
	}

	_, err := c.sdk.VolumeCreate(ctx, volumetypes.CreateOptions{Name: name})
	return err
}

// RemoveVolume removes a named Docker volume.
func (c *Client) RemoveVolume(ctx context.Context, name string) error {
	if c.sdk == nil {
		return errors.New("docker client is nil")
	}
	if name == "" {
		return errors.New("volume name is required")
	}

	return c.sdk.VolumeRemove(ctx, name, false)
}

// ListOrcaContainers lists Docker containers with the orca- prefix and parses their names.
func (c *Client) ListOrcaContainers(ctx context.Context) ([]ContainerInfo, error) {
	if c.sdk == nil {
		return nil, errors.New("docker client is nil")
	}

	containers, err := c.sdk.ContainerList(ctx, containertypes.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", "^/"+orcaNamePrefix)),
	})
	if err != nil {
		return nil, err
	}

	infos := make([]ContainerInfo, 0, len(containers))
	for _, container := range containers {
		info, ok := parseContainerSummary(container)
		if ok {
			configPath := ""
			switch info.Kind {
			case ContainerKindPrimary:
				configPath = PostgresAppliedConfigRelativePath
			case ContainerKindPgBouncer:
				configPath = PgBouncerConfigRelativePath
			}
			if configPath != "" {
				config, err := readConfig(c.dataRoot, info.ClusterID, configPath)
				if err != nil {
					return nil, fmt.Errorf("read config for cluster %q: %w", info.ClusterID, err)
				}
				info.Config = config
			}
			if info.Kind == ContainerKindPrimary {
				backupConfig, err := readConfig(c.dataRoot, info.ClusterID, PgBackRestAppliedConfigRelativePath)
				if err != nil {
					return nil, fmt.Errorf("read pgBackRest config for cluster %q: %w", info.ClusterID, err)
				}
				info.BackupConfig = backupConfig
			}
			infos = append(infos, info)
		}
	}

	return infos, nil
}

// ListOrcaVolumes lists named Orca cluster data volumes.
func (c *Client) ListOrcaVolumes(ctx context.Context) ([]VolumeInfo, error) {
	if c.sdk == nil {
		return nil, errors.New("docker client is nil")
	}
	sdk, ok := c.sdk.(volumeListSDKClient)
	if !ok {
		return nil, errors.New("docker client does not support volume listing")
	}
	volumes, err := sdk.VolumeList(ctx, volumetypes.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]VolumeInfo, 0, len(volumes.Volumes))
	for _, volume := range volumes.Volumes {
		if volume == nil || !strings.HasPrefix(volume.Name, orcaNamePrefix) {
			continue
		}
		clusterID, ok := strings.CutSuffix(strings.TrimPrefix(volume.Name, orcaNamePrefix), "-data")
		if ok && clusterID != "" {
			result = append(result, VolumeInfo{Name: volume.Name, ClusterID: clusterID})
		}
	}
	return result, nil
}

// ContainerName returns the Docker container name for an Orca container spec.
func ContainerName(spec ContainerSpec) (string, error) {
	if spec.ClusterID == "" {
		return "", errors.New("cluster ID is required")
	}

	switch spec.Kind {
	case ContainerKindPrimary:
		return fmt.Sprintf("orca-%s-primary", spec.ClusterID), nil
	case ContainerKindReplica:
		if spec.ReplicaID == "" {
			return "", errors.New("replica ID is required")
		}
		return fmt.Sprintf("orca-%s-replica-%s", spec.ClusterID, spec.ReplicaID), nil
	case ContainerKindPgBouncer:
		return fmt.Sprintf("orca-%s-pgbouncer", spec.ClusterID), nil
	case ContainerKindPgBackRest:
		return fmt.Sprintf("orca-%s-pgbackrest", spec.ClusterID), nil
	default:
		return "", fmt.Errorf("unknown container kind %q", spec.Kind)
	}
}

// VolumeName returns the explicit Docker volume name for an Orca cluster.
func VolumeName(clusterID string) string {
	return fmt.Sprintf("orca-%s-data", clusterID)
}

// VolumeMountPath returns the in-container data mount path for an Orca cluster.
func VolumeMountPath(clusterID string) string {
	return fmt.Sprintf("%s/%s", dataRoot, clusterID)
}

func writeConfigMount(root, clusterID string, config *ConfigMount) (mount.Mount, error) {
	if config == nil {
		return mount.Mount{}, errors.New("config mount is required")
	}
	if config.ContainerPath == "" {
		return mount.Mount{}, errors.New("config container path is required")
	}

	hostPath, err := configHostPath(root, clusterID, config.RelativePath)
	if err != nil {
		return mount.Mount{}, err
	}
	if err := os.MkdirAll(filepath.Dir(hostPath), 0o755); err != nil {
		return mount.Mount{}, fmt.Errorf("create config directory: %w", err)
	}

	temporary, err := os.CreateTemp(filepath.Dir(hostPath), ".orca-config-*")
	if err != nil {
		return mount.Mount{}, fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return mount.Mount{}, fmt.Errorf("set config permissions: %w", err)
	}
	if _, err := temporary.WriteString(config.Content); err != nil {
		temporary.Close()
		return mount.Mount{}, fmt.Errorf("write config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return mount.Mount{}, fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(temporaryPath, hostPath); err != nil {
		return mount.Mount{}, fmt.Errorf("install config: %w", err)
	}

	return mount.Mount{
		Type:     mount.TypeBind,
		Source:   filepath.Dir(hostPath),
		Target:   filepath.Dir(config.ContainerPath),
		ReadOnly: true,
	}, nil
}

func writeConfig(root, clusterID string, config *ConfigMount) error {
	if config == nil {
		return errors.New("config mount is required")
	}
	hostPath, err := configHostPath(root, clusterID, config.RelativePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(hostPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(hostPath, []byte(config.Content), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func readConfig(root, clusterID, relativePath string) (string, error) {
	hostPath, err := configHostPath(root, clusterID, relativePath)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(hostPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func configHostPath(root, clusterID, relativePath string) (string, error) {
	if root == "" {
		return "", errors.New("data root is required")
	}
	if clusterID == "" || filepath.Base(clusterID) != clusterID {
		return "", errors.New("cluster ID must be a path segment")
	}
	cleanPath := filepath.Clean(relativePath)
	if cleanPath == "." || filepath.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", errors.New("config path must be relative to the cluster data directory")
	}
	return filepath.Join(root, clusterID, cleanPath), nil
}

func parseContainerSummary(container dockertypes.Container) (ContainerInfo, bool) {
	for _, rawName := range container.Names {
		name := strings.TrimPrefix(rawName, "/")
		if !strings.HasPrefix(name, orcaNamePrefix) {
			continue
		}

		parsed, ok := parseContainerLabels(container.Labels)
		if !ok {
			parsed, ok = parseContainerName(name)
			if !ok {
				continue
			}
		}

		parsed.ID = container.ID
		parsed.Name = name
		parsed.Image = container.Image
		parsed.Status = containerStatus(container.State)

		return parsed, true
	}

	return ContainerInfo{}, false
}

func parseContainerLabels(labels map[string]string) (ContainerInfo, bool) {
	clusterID := labels["orca.cluster_id"]
	kind := ContainerKind(labels["orca.kind"])
	if clusterID == "" {
		return ContainerInfo{}, false
	}

	switch kind {
	case ContainerKindPrimary, ContainerKindPgBouncer:
		return ContainerInfo{ClusterID: clusterID, Kind: kind}, true
	case ContainerKindReplica:
		replicaID := labels["orca.replica_id"]
		if replicaID == "" {
			return ContainerInfo{}, false
		}
		return ContainerInfo{ClusterID: clusterID, Kind: kind, ReplicaID: replicaID}, true
	default:
		return ContainerInfo{}, false
	}
}

func parseContainerName(name string) (ContainerInfo, bool) {
	if !strings.HasPrefix(name, orcaNamePrefix) {
		return ContainerInfo{}, false
	}

	withoutPrefix := strings.TrimPrefix(name, orcaNamePrefix)
	if clusterID, ok := strings.CutSuffix(withoutPrefix, "-primary"); ok && clusterID != "" {
		return ContainerInfo{ClusterID: clusterID, Kind: ContainerKindPrimary}, true
	}
	if clusterID, ok := strings.CutSuffix(withoutPrefix, "-pgbouncer"); ok && clusterID != "" {
		return ContainerInfo{ClusterID: clusterID, Kind: ContainerKindPgBouncer}, true
	}

	if index := strings.LastIndex(withoutPrefix, "-replica-"); index > 0 {
		clusterID := withoutPrefix[:index]
		replicaID := withoutPrefix[index+len("-replica-"):]
		if replicaID == "" {
			return ContainerInfo{}, false
		}

		return ContainerInfo{ClusterID: clusterID, Kind: ContainerKindReplica, ReplicaID: replicaID}, true
	}

	return ContainerInfo{}, false
}

func containerStatus(state string) string {
	switch state {
	case "running":
		return "running"
	case "created", "exited", "dead", "paused", "restarting", "removing":
		return "stopped"
	default:
		return "unknown"
	}
}
