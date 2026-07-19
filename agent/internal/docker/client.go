package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	orcaNamePrefix = "orca-"
	dataRoot       = "/var/orca/data"
)

type sdkClient interface {
	ContainerCreate(ctx context.Context, config *containertypes.Config, hostConfig *containertypes.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (containertypes.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options containertypes.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options containertypes.StopOptions) error
	ContainerRemove(ctx context.Context, containerID string, options containertypes.RemoveOptions) error
	ContainerList(ctx context.Context, options containertypes.ListOptions) ([]dockertypes.Container, error)
	ImagePull(ctx context.Context, ref string, options imagetypes.PullOptions) (io.ReadCloser, error)
	VolumeCreate(ctx context.Context, options volumetypes.CreateOptions) (volumetypes.Volume, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
}

// Client implements DockerClient using the official Docker SDK for Go.
type Client struct {
	sdk sdkClient
}

// NewClient creates a Docker SDK-backed Client from environment configuration.
func NewClient() (*Client, error) {
	sdk, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return NewClientWithSDK(sdk), nil
}

// NewClientWithSDK creates a Client using the provided Docker SDK-compatible client.
func NewClientWithSDK(sdk sdkClient) *Client {
	return &Client{sdk: sdk}
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

		hostConfig.Mounts = []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: volumeName,
			Target: VolumeMountPath(spec.ClusterID),
		}}
	}

	config := &containertypes.Config{
		Image:  spec.Image,
		Env:    spec.Env,
		Cmd:    spec.Command,
		Labels: labels,
	}
	created, err := c.sdk.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if errdefs.IsNotFound(err) {
		if err := c.pullImage(ctx, spec.Image); err != nil {
			return "", err
		}
		created, err = c.sdk.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	}
	if err != nil {
		return "", err
	}

	return created.ID, nil
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
			infos = append(infos, info)
		}
	}

	return infos, nil
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
