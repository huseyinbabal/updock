// Package docker provides a high-level client for interacting with the Docker daemon.
//
// It wraps the official Docker SDK and exposes container lifecycle operations
// needed by Updock: listing, inspecting, pulling images, stopping, recreating,
// and executing lifecycle hook commands inside containers.
//
// # Connection
//
// By default, the client connects via the local Unix socket at
// /var/run/docker.sock. Remote Docker hosts are supported via TCP endpoints:
//
//	client, err := docker.NewClient("tcp://10.0.1.2:2375", false)
//
// # Container Recreation
//
// When updating a container, Updock performs a safe recreation with automatic
// rollback. The sequence is:
//
//  1. Inspect the old container to capture its full configuration.
//  2. Execute pre-update lifecycle hook (if enabled).
//  3. Stop the old container (respecting custom stop signals).
//  4. Rename the old container to free up the name.
//  5. Create a new container with the updated image but identical config.
//  6. Start the new container.
//  7. Execute post-update lifecycle hook (if enabled).
//  8. Remove the old container.
//
// If any step fails after stopping the old container, the client automatically
// rolls back by renaming and restarting the old container.
package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/huseyinbabal/updock/internal/logger"
)

// DockerClient defines the interface for all Docker operations that Updock needs.
// This interface enables mock-based testing of all components that interact
// with the Docker daemon (updater, API server, scheduler) without requiring
// a live Docker daemon.
type DockerClient interface {
	// Ping verifies connectivity to the Docker daemon.
	Ping(ctx context.Context) error

	// ListContainers returns containers matching the specified criteria.
	ListContainers(ctx context.Context, includeStopped, includeRestarting bool) ([]ContainerInfo, error)

	// InspectContainer returns detailed information about a container.
	InspectContainer(ctx context.Context, id string) (*ContainerInfo, error)

	// PullImage pulls an image and returns the new image ID.
	PullImage(ctx context.Context, ref string, registryAuth string) (string, error)

	// GetImageDigest returns the repository digest of a local image.
	GetImageDigest(ctx context.Context, imageID string) (string, error)

	// StopContainer stops a running container with optional custom signal.
	StopContainer(ctx context.Context, id string, timeout time.Duration, customSignal string) error

	// StartContainer starts a stopped container.
	StartContainer(ctx context.Context, id string) error

	// RemoveContainer forcefully removes a container.
	RemoveContainer(ctx context.Context, id string, removeVolumes bool) error

	// RemoveImage removes an image by ID or reference.
	RemoveImage(ctx context.Context, imageID string) error

	// ExecCommand executes a command inside a running container.
	ExecCommand(ctx context.Context, containerID string, cmd string, timeout time.Duration) (string, error)

	// RecreateContainer performs a safe container recreation with rollback.
	RecreateContainer(ctx context.Context, id string, newImage string, stopTimeout time.Duration, customSignal string, removeVolumes bool) (string, error)

	// GetDependencies returns container names this container depends on.
	GetDependencies(ctx context.Context, containerID string) ([]string, error)

	// Close releases resources held by the client.
	Close() error

	// WaitForHealthy waits for a container to become healthy.
	WaitForHealthy(ctx context.Context, containerID string, timeout time.Duration) error
}

// Client wraps the Docker Engine API client and implements the [DockerClient] interface.
type Client struct {
	api dockerclient.APIClient
}

// Compile-time check that Client implements DockerClient.
var _ DockerClient = (*Client)(nil)

// ContainerInfo holds the metadata Updock needs about a running container.
// It is a flattened, serializable view of the Docker container state.
type ContainerInfo struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	ImageID       string            `json:"image_id"`
	Status        string            `json:"status"`
	State         string            `json:"state"`
	Created       time.Time         `json:"created"`
	Labels        map[string]string `json:"labels"`
	Env           []string          `json:"env,omitempty"`
	Ports         []PortBinding     `json:"ports,omitempty"`
	Mounts        []Mount           `json:"mounts,omitempty"`
	Networks      []string          `json:"networks,omitempty"`
	RestartPolicy string            `json:"restart_policy,omitempty"`
}

// PortBinding represents a mapping between a host port and a container port.
type PortBinding struct {
	HostPort      string `json:"host_port"`
	ContainerPort string `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// Mount represents a volume or bind mount attached to a container.
type Mount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Type        string `json:"type"`
	ReadOnly    bool   `json:"read_only"`
}

// NewClient creates a new Docker client connected to the specified host.
//
// If host is empty or the default Unix socket path, the client uses environment
// variables (DOCKER_HOST, DOCKER_TLS_VERIFY, etc.) for configuration.
// API version negotiation is always enabled to support older Docker daemons.
//
// When tlsVerify is true, the client requires a valid TLS connection to the
// Docker daemon using certificates from the standard Docker TLS paths.
func NewClient(host string, tlsVerify bool) (*Client, error) {
	opts := []dockerclient.Opt{
		dockerclient.WithAPIVersionNegotiation(),
	}

	if host != "" && host != "unix:///var/run/docker.sock" {
		opts = append(opts, dockerclient.WithHost(host))
	} else {
		opts = append(opts, dockerclient.FromEnv)
	}

	if tlsVerify {
		opts = append(opts, dockerclient.WithTLSClientConfigFromEnv())
	}

	api, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}

	return &Client{api: api}, nil
}

// Ping verifies connectivity to the Docker daemon.
// Returns nil if the daemon is reachable, or an error describing the failure.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.api.Ping(ctx)
	return err
}

// ListContainers returns containers matching the specified criteria.
//
// When includeStopped is true, exited and created containers are also returned.
// When includeRestarting is true, containers in the "restarting" state are included.
// By default only running containers are returned.
func (c *Client) ListContainers(ctx context.Context, includeStopped, includeRestarting bool) ([]ContainerInfo, error) {
	listAll := includeStopped // All=true includes non-running containers
	containers, err := c.api.ContainerList(ctx, container.ListOptions{All: listAll})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	var result []ContainerInfo
	for _, ctr := range containers {
		// Filter restarting containers if not requested
		if ctr.State == "restarting" && !includeRestarting {
			continue
		}

		name := ""
		if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(ctr.Names[0], "/")
		}

		info := ContainerInfo{
			ID:      ctr.ID,
			Name:    name,
			Image:   ctr.Image,
			ImageID: ctr.ImageID,
			Status:  ctr.Status,
			State:   ctr.State,
			Created: time.Unix(ctr.Created, 0),
			Labels:  ctr.Labels,
		}

		result = append(result, info)
	}

	return result, nil
}

// InspectContainer returns detailed information about a container by ID or name.
// This includes environment variables, port bindings, mounts, networks, and labels.
func (c *Client) InspectContainer(ctx context.Context, id string) (*ContainerInfo, error) {
	ctr, err := c.api.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspecting container %s: %w", id, err)
	}

	name := strings.TrimPrefix(ctr.Name, "/")

	info := &ContainerInfo{
		ID:      ctr.ID,
		Name:    name,
		Image:   ctr.Config.Image,
		ImageID: ctr.Image,
		State:   ctr.State.Status,
		Labels:  ctr.Config.Labels,
		Env:     ctr.Config.Env,
	}

	if ctr.State.Running {
		info.Status = "running"
	} else {
		info.Status = ctr.State.Status
	}

	if ctr.Created != "" {
		if t, err := time.Parse(time.RFC3339Nano, ctr.Created); err == nil {
			info.Created = t
		}
	}

	// Extract port bindings from HostConfig
	if ctr.HostConfig != nil {
		for port, bindings := range ctr.HostConfig.PortBindings {
			for _, b := range bindings {
				info.Ports = append(info.Ports, PortBinding{
					HostPort:      b.HostPort,
					ContainerPort: port.Port(),
					Protocol:      port.Proto(),
				})
			}
		}
		info.RestartPolicy = string(ctr.HostConfig.RestartPolicy.Name)
	}

	// Extract volume mounts
	for _, m := range ctr.Mounts {
		info.Mounts = append(info.Mounts, Mount{
			Source:      m.Source,
			Destination: m.Destination,
			Type:        string(m.Type),
			ReadOnly:    !m.RW,
		})
	}

	// Extract network names
	if ctr.NetworkSettings != nil {
		for netName := range ctr.NetworkSettings.Networks {
			info.Networks = append(info.Networks, netName)
		}
	}

	return info, nil
}

// PullImage pulls the latest version of an image from the registry.
// It reads the full pull output stream to completion and returns the
// resulting local image ID. The registryAuth parameter is a base64-encoded
// JSON auth config for private registries (empty string for public images).
func (c *Client) PullImage(ctx context.Context, ref string, registryAuth string) (string, error) {
	logger.Info().Msgf("Pulling image: %s", ref)

	pullOpts := image.PullOptions{}
	if registryAuth != "" {
		pullOpts.RegistryAuth = registryAuth
	}

	reader, err := c.api.ImagePull(ctx, ref, pullOpts)
	if err != nil {
		return "", fmt.Errorf("pulling image %s: %w", ref, err)
	}
	defer func() { _ = reader.Close() }()

	// Consume the pull output stream, checking for errors
	decoder := json.NewDecoder(reader)
	for {
		var event map[string]interface{}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("reading pull output: %w", err)
		}
		if errMsg, ok := event["error"]; ok {
			return "", fmt.Errorf("pull error: %v", errMsg)
		}
	}

	// Retrieve the image ID of the newly pulled image
	inspect, err := c.api.ImageInspect(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("inspecting pulled image %s: %w", ref, err)
	}

	return inspect.ID, nil
}

// GetImageDigest returns the repository digest of a local image.
// If no digest is available (e.g. locally built images), the image ID is returned.
func (c *Client) GetImageDigest(ctx context.Context, imageID string) (string, error) {
	inspect, err := c.api.ImageInspect(ctx, imageID)
	if err != nil {
		return "", fmt.Errorf("inspecting image %s: %w", imageID, err)
	}

	if len(inspect.RepoDigests) > 0 {
		return inspect.RepoDigests[0], nil
	}

	return inspect.ID, nil
}

// StopContainer sends a stop signal to a container and waits up to the given
// timeout for it to exit. If customSignal is non-empty, that signal is sent
// instead of the default SIGTERM (e.g. "SIGHUP", "SIGQUIT").
func (c *Client) StopContainer(ctx context.Context, id string, timeout time.Duration, customSignal string) error {
	// Send custom stop signal if specified
	if customSignal != "" {
		sig := parseSignal(customSignal)
		if sig != 0 {
			logger.Info().Msgf("Sending %s to container %s", customSignal, shortID(id))
			if err := c.api.ContainerKill(ctx, id, customSignal); err != nil {
				logger.Warn().Msgf("Failed to send custom signal %s: %v", customSignal, err)
			}
		}
	}

	logger.Info().Msgf("Stopping container: %s", shortID(id))
	timeoutSec := int(timeout.Seconds())
	return c.api.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeoutSec})
}

// StartContainer starts a stopped container.
func (c *Client) StartContainer(ctx context.Context, id string) error {
	logger.Info().Msgf("Starting container: %s", shortID(id))
	return c.api.ContainerStart(ctx, id, container.StartOptions{})
}

// RemoveContainer forcefully removes a container.
// If removeVolumes is true, anonymous volumes attached to the container are also removed.
func (c *Client) RemoveContainer(ctx context.Context, id string, removeVolumes bool) error {
	logger.Info().Msgf("Removing container: %s", shortID(id))
	return c.api.ContainerRemove(ctx, id, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: removeVolumes,
	})
}

// RemoveImage removes an image by ID or reference.
// Child images are pruned automatically.
func (c *Client) RemoveImage(ctx context.Context, imageID string) error {
	logger.Info().Msgf("Removing old image: %s", shortID(imageID))
	_, err := c.api.ImageRemove(ctx, imageID, image.RemoveOptions{PruneChildren: true})
	return err
}

// ExecCommand executes a shell command inside a running container.
// It returns the combined stdout/stderr output of the command.
// The timeout parameter limits how long the command can run; 0 means no timeout.
//
// This is used for lifecycle hooks (pre-check, pre-update, post-update, post-check).
// The command is executed via "sh -c <cmd>" and requires the container to have
// a /bin/sh executable.
func (c *Client) ExecCommand(ctx context.Context, containerID string, cmd string, timeout time.Duration) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	logger.Debug().Msgf("Executing command in container %s: %s", shortID(containerID), cmd)

	execConfig := container.ExecOptions{
		Cmd:          []string{"sh", "-c", cmd},
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := c.api.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("creating exec in container %s: %w", shortID(containerID), err)
	}

	attachResp, err := c.api.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("attaching to exec in container %s: %w", shortID(containerID), err)
	}
	defer attachResp.Close()

	var output bytes.Buffer
	_, _ = io.Copy(&output, attachResp.Reader)

	// Check exit code
	inspectResp, err := c.api.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return output.String(), fmt.Errorf("inspecting exec result: %w", err)
	}

	if inspectResp.ExitCode != 0 && inspectResp.ExitCode != 75 { // 75 = EX_TEMPFAIL
		return output.String(), fmt.Errorf("command exited with code %d", inspectResp.ExitCode)
	}

	return output.String(), nil
}

// RecreateContainer performs a safe container recreation with automatic rollback.
//
// The process preserves the container's name, configuration, host config,
// and network settings while updating the image reference. If creation or
// startup of the new container fails, the old container is automatically
// restored.
//
// Parameters:
//   - id: the ID of the container to recreate
//   - newImage: the image reference (name:tag) for the new container
//   - stopTimeout: how long to wait for the container to stop gracefully
//   - customSignal: optional signal to send before stopping (e.g. "SIGHUP")
//   - removeVolumes: whether to remove anonymous volumes from the old container
func (c *Client) RecreateContainer(ctx context.Context, id string, newImage string, stopTimeout time.Duration, customSignal string, removeVolumes bool) (string, error) {
	// Step 1: Inspect old container to capture full configuration
	oldContainer, err := c.api.ContainerInspect(ctx, id)
	if err != nil {
		return "", fmt.Errorf("inspecting container for recreation: %w", err)
	}

	containerName := strings.TrimPrefix(oldContainer.Name, "/")

	// Step 2: Stop old container (with optional custom signal)
	if err := c.StopContainer(ctx, id, stopTimeout, customSignal); err != nil {
		return "", fmt.Errorf("stopping container for recreation: %w", err)
	}

	// Step 3: Rename old container to free the name for the new one
	backupName := containerName + "_updock_old"
	if err := c.api.ContainerRename(ctx, id, backupName); err != nil {
		return "", fmt.Errorf("renaming old container: %w", err)
	}

	// Step 4: Create new container with updated image but same config
	newConfig := oldContainer.Config
	newConfig.Image = newImage

	var networkingConfig *network.NetworkingConfig
	if oldContainer.NetworkSettings != nil && len(oldContainer.NetworkSettings.Networks) > 0 {
		networkingConfig = &network.NetworkingConfig{
			EndpointsConfig: oldContainer.NetworkSettings.Networks,
		}
	}

	resp, err := c.api.ContainerCreate(ctx, newConfig, oldContainer.HostConfig, networkingConfig, nil, containerName)
	if err != nil {
		// Rollback: restore old container name and restart it
		logger.Warn().Msgf("Rolling back: failed to create new container: %v", err)
		_ = c.api.ContainerRename(ctx, id, containerName)
		_ = c.api.ContainerStart(ctx, id, container.StartOptions{})
		return "", fmt.Errorf("creating new container: %w", err)
	}

	// Step 5: Start the new container
	if err := c.api.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Rollback: remove failed container and restore old one
		logger.Warn().Msgf("Rolling back: failed to start new container: %v", err)
		_ = c.api.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		_ = c.api.ContainerRename(ctx, id, containerName)
		_ = c.api.ContainerStart(ctx, id, container.StartOptions{})
		return "", fmt.Errorf("starting new container: %w", err)
	}

	// Step 6: Remove old container
	if err := c.api.ContainerRemove(ctx, id, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: removeVolumes,
	}); err != nil {
		logger.Warn().Msgf("Failed to remove old container %s: %v", shortID(id), err)
	}

	logger.Info().Msgf("Successfully recreated container %s with new image", containerName)
	return resp.ID, nil
}

// GetDependencies returns the list of container names that the given container
// depends on, as specified by the LabelDependsOn label (comma-separated).
//
// Also detects implicit dependencies via network_mode: service:<container>.
func (c *Client) GetDependencies(ctx context.Context, containerID string) ([]string, error) {
	ctr, err := c.api.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	var deps []string

	// Explicit dependencies via label
	if depLabel, ok := ctr.Config.Labels["com.updock.depends-on"]; ok && depLabel != "" {
		for _, dep := range strings.Split(depLabel, ",") {
			dep = strings.TrimSpace(dep)
			if dep != "" {
				deps = append(deps, dep)
			}
		}
	}

	// Implicit dependency via network_mode: service:<container>
	if ctr.HostConfig != nil {
		netMode := string(ctr.HostConfig.NetworkMode)
		if strings.HasPrefix(netMode, "container:") {
			depID := strings.TrimPrefix(netMode, "container:")
			depInfo, err := c.api.ContainerInspect(ctx, depID)
			if err == nil {
				deps = append(deps, strings.TrimPrefix(depInfo.Name, "/"))
			}
		}
	}

	return deps, nil
}

// Close releases resources held by the Docker client.
func (c *Client) Close() error {
	return c.api.Close()
}

// WaitForHealthy waits for a container to become healthy or running.
// It polls the container state every second up to the timeout.
// This is useful after starting a new container to verify it started correctly.
func (c *Client) WaitForHealthy(ctx context.Context, containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ctr, err := c.api.ContainerInspect(ctx, containerID)
		if err != nil {
			return err
		}

		// No healthcheck configured - just check if running
		if ctr.State.Health == nil {
			if ctr.State.Running {
				return nil
			}
		} else {
			if ctr.State.Health.Status == types.Healthy {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	return fmt.Errorf("container %s did not become healthy within %s", shortID(containerID), timeout)
}

// shortID returns the first 12 characters of a container or image ID.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// parseSignal converts a signal name (e.g. "SIGHUP") to a syscall.Signal.
// Returns 0 if the signal name is not recognized.
func parseSignal(name string) syscall.Signal {
	name = strings.ToUpper(strings.TrimSpace(name))
	signals := map[string]syscall.Signal{
		"SIGHUP":  syscall.SIGHUP,
		"SIGINT":  syscall.SIGINT,
		"SIGQUIT": syscall.SIGQUIT,
		"SIGTERM": syscall.SIGTERM,
		"SIGUSR1": syscall.SIGUSR1,
		"SIGUSR2": syscall.SIGUSR2,
	}
	if sig, ok := signals[name]; ok {
		return sig
	}
	return 0
}
