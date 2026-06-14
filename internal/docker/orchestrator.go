// Package docker wraps the official Docker Engine SDK with the narrow set of
// operations Porque needs to manage Minecraft server containers and volumes.
package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

// ManagedLabel marks every container/volume Porque creates, so the worker can
// filter Docker events to only resources it owns.
const ManagedLabel = "porque.managed"

// Orchestrator is a thin wrapper over the Docker Engine API.
type Orchestrator struct {
	cli *client.Client
}

// New constructs an Orchestrator from the ambient Docker environment
// (DOCKER_HOST or the default socket), negotiating the API version.
func New() (*Orchestrator, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Orchestrator{cli: cli}, nil
}

// Ping verifies connectivity to the Docker daemon.
func (o *Orchestrator) Ping(ctx context.Context) error {
	_, err := o.cli.Ping(ctx)
	return err
}

// Close releases the underlying client.
func (o *Orchestrator) Close() error { return o.cli.Close() }

// CreateOpts describes a Minecraft server container to create.
type CreateOpts struct {
	Name       string            // container name, e.g. mc-server-<name>
	Image      string            // e.g. itzg/minecraft-server
	Env        []string          // itzg configuration
	MemoryMB   int               // hard memory limit
	CPUCores   float64           // CPU quota in cores (-> NanoCPUs)
	VolumeName string            // docker named volume mounted at /data
	Labels     map[string]string // additional labels (ManagedLabel added automatically)
}

func mcPort() nat.Port   { return nat.Port(fmt.Sprintf("%d/tcp", 25565)) }
func rconPort() nat.Port { return nat.Port(fmt.Sprintf("%d/tcp", 25575)) }

// pidsLimit caps the number of processes a server container may spawn.
func pidsLimit() *int64 { n := int64(512); return &n }

// normalizeBindSource converts a host bind-mount path to forward slashes, which
// Docker Desktop on Windows resolves consistently whether the request comes
// from the host CLI or via the mounted docker.sock from inside a container.
// Mixed separators (e.g. "C:\\Users\\x/data") otherwise resolve to a different
// location, so sibling containers and the host see divergent data.
func normalizeBindSource(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

// EnsureVolume creates a named volume if it does not already exist.
func (o *Orchestrator) EnsureVolume(ctx context.Context, name string) error {
	_, err := o.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:   name,
		Labels: map[string]string{ManagedLabel: "true"},
	})
	if err != nil {
		return fmt.Errorf("create volume %s: %w", name, err)
	}
	return nil
}

// RemoveVolume deletes a named volume, forcing if necessary.
func (o *Orchestrator) RemoveVolume(ctx context.Context, name string) error {
	if err := o.cli.VolumeRemove(ctx, name, true); err != nil {
		return fmt.Errorf("remove volume %s: %w", name, err)
	}
	return nil
}

// PullImage pulls an image and waits for completion.
func (o *Orchestrator) PullImage(ctx context.Context, ref string) error {
	rc, err := o.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	defer rc.Close()
	// Draining the stream is required for the pull to finish.
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	return nil
}

// CreateMCContainer creates (but does not start) a Minecraft server container.
// The game port is published on a random host port; RCON is published only on
// loopback for backup/console use.
func (o *Orchestrator) CreateMCContainer(ctx context.Context, opts CreateOpts) (string, error) {
	labels := map[string]string{ManagedLabel: "true"}
	for k, v := range opts.Labels {
		labels[k] = v
	}

	cfg := &container.Config{
		Image:  opts.Image,
		Env:    opts.Env,
		Labels: labels,
		ExposedPorts: nat.PortSet{
			mcPort():   struct{}{},
			rconPort(): struct{}{},
		},
	}

	memBytes := int64(opts.MemoryMB) * 1024 * 1024
	mountType := mount.TypeVolume
	source := opts.VolumeName
	if strings.Contains(opts.VolumeName, "/") || strings.Contains(opts.VolumeName, "\\") {
		mountType = mount.TypeBind
		source = normalizeBindSource(opts.VolumeName)
	}

	hostCfg := &container.HostConfig{
		Mounts: []mount.Mount{{
			Type:   mountType,
			Source: source,
			Target: "/data",
		}},
		PortBindings: nat.PortMap{
			mcPort():   []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: ""}},
			rconPort(): []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: ""}},
		},
		Resources: container.Resources{
			Memory:     memBytes,
			MemorySwap: memBytes, // == Memory disables swap (no disk thrashing)
			NanoCPUs:   int64(opts.CPUCores * 1e9),
			PidsLimit:  pidsLimit(), // cap processes to blunt fork bombs
		},
		// Hardening: the worker (not Docker) handles restarts; block privilege
		// escalation inside the container.
		RestartPolicy: container.RestartPolicy{Name: "no"},
		SecurityOpt:   []string{"no-new-privileges:true"},
	}

	resp, err := o.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, opts.Name)
	if err != nil {
		return "", fmt.Errorf("create container %s: %w", opts.Name, err)
	}
	return resp.ID, nil
}

// StartContainer starts a created container.
func (o *Orchestrator) StartContainer(ctx context.Context, id string) error {
	if err := o.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return fmt.Errorf("start %s: %w", id, err)
	}
	return nil
}

// StopContainer stops a container, allowing itzg up to timeout for a graceful
// world save before SIGKILL.
func (o *Orchestrator) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	secs := int(timeout.Seconds())
	if err := o.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &secs}); err != nil {
		return fmt.Errorf("stop %s: %w", id, err)
	}
	return nil
}

// RestartContainer restarts a container with a graceful stop timeout.
func (o *Orchestrator) RestartContainer(ctx context.Context, id string, timeout time.Duration) error {
	secs := int(timeout.Seconds())
	if err := o.cli.ContainerRestart(ctx, id, container.StopOptions{Timeout: &secs}); err != nil {
		return fmt.Errorf("restart %s: %w", id, err)
	}
	return nil
}

// RemoveContainer force-removes a container.
func (o *Orchestrator) RemoveContainer(ctx context.Context, id string) error {
	err := o.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
	if err != nil {
		return fmt.Errorf("remove %s: %w", id, err)
	}
	return nil
}

// RemoveContainerByNameQuiet force-removes a container by name, ignoring the
// "no such container" case. Used to clear stale containers before (re)creating.
func (o *Orchestrator) RemoveContainerByNameQuiet(ctx context.Context, name string) {
	err := o.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	if err != nil && !client.IsErrNotFound(err) {
		// Best-effort cleanup; a real conflict will surface on the next create.
		return
	}
}

// RunSidecar creates and starts a sidecar container that shares the network
// namespace of netnsContainerID (so the sidecar reaches the target on
// 127.0.0.1). Used for the Playit agent, which forwards 127.0.0.1:25565.
func (o *Orchestrator) RunSidecar(ctx context.Context, name, img string, env []string, netnsContainerID string, labels map[string]string) (string, error) {
	lbl := map[string]string{ManagedLabel: "true"}
	for k, v := range labels {
		lbl[k] = v
	}
	cfg := &container.Config{Image: img, Env: env, Labels: lbl}
	hostCfg := &container.HostConfig{
		NetworkMode:   container.NetworkMode("container:" + netnsContainerID),
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
	}
	o.RemoveContainerByNameQuiet(ctx, name)
	resp, err := o.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("create sidecar %s: %w", name, err)
	}
	if err := o.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = o.RemoveContainer(ctx, resp.ID)
		return "", fmt.Errorf("start sidecar %s: %w", name, err)
	}
	return resp.ID, nil
}

// ExecCapture runs a command inside a container and returns its combined
// stdout+stderr output, erroring on a non-zero exit code.
func (o *Orchestrator) ExecCapture(ctx context.Context, containerID string, cmd []string) (string, error) {
	created, err := o.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("exec create: %w", err)
	}
	attach, err := o.cli.ContainerExecAttach(ctx, created.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("exec attach: %w", err)
	}
	defer attach.Close()

	var out bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &out, attach.Reader); err != nil {
		return out.String(), fmt.Errorf("exec read: %w", err)
	}
	insp, err := o.cli.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return out.String(), fmt.Errorf("exec inspect: %w", err)
	}
	if insp.ExitCode != 0 {
		return out.String(), fmt.Errorf("command %v exited %d: %s", cmd, insp.ExitCode, out.String())
	}
	return out.String(), nil
}

// CopyFromContainer returns a tar stream of srcPath inside the container. The
// archive entries are rooted at the base name of srcPath (e.g. "data/...").
func (o *Orchestrator) CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, error) {
	rc, _, err := o.cli.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return nil, fmt.Errorf("copy from %s: %w", containerID, err)
	}
	return rc, nil
}

// CopyToContainer extracts an (uncompressed) tar stream into dstPath inside the
// container.
func (o *Orchestrator) CopyToContainer(ctx context.Context, containerID, dstPath string, tarStream io.Reader) error {
	err := o.cli.CopyToContainer(ctx, containerID, dstPath, tarStream, container.CopyToContainerOptions{})
	if err != nil {
		return fmt.Errorf("copy to %s: %w", containerID, err)
	}
	return nil
}

// RunHelper creates and starts a short-lived helper container with a named
// volume mounted at mountPath, returning its id. Callers remove it when done.
func (o *Orchestrator) RunHelper(ctx context.Context, name, img, volumeName, mountPath string, cmd []string) (string, error) {
	cfg := &container.Config{
		Image:  img,
		Cmd:    cmd,
		Labels: map[string]string{ManagedLabel: "true"},
	}
	// Mirror CreateMCContainer: a source containing a path separator is a host
	// bind mount, otherwise it's a Docker named volume.
	mountType := mount.TypeVolume
	source := volumeName
	if strings.Contains(volumeName, "/") || strings.Contains(volumeName, "\\") {
		mountType = mount.TypeBind
		source = normalizeBindSource(volumeName)
	}
	hostCfg := &container.HostConfig{
		Mounts: []mount.Mount{{
			Type:   mountType,
			Source: source,
			Target: mountPath,
		}},
		AutoRemove: false,
	}
	o.RemoveContainerByNameQuiet(ctx, name)
	resp, err := o.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("create helper %s: %w", name, err)
	}
	if err := o.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = o.RemoveContainer(ctx, resp.ID)
		return "", fmt.Errorf("start helper %s: %w", name, err)
	}
	return resp.ID, nil
}

// InspectPorts returns the host ports mapped to the game and RCON ports.
func (o *Orchestrator) InspectPorts(ctx context.Context, id string) (mcHostPort, rconHostPort int, err error) {
	insp, err := o.cli.ContainerInspect(ctx, id)
	if err != nil {
		return 0, 0, fmt.Errorf("inspect %s: %w", id, err)
	}
	if insp.NetworkSettings == nil {
		return 0, 0, fmt.Errorf("inspect %s: no network settings", id)
	}
	mcHostPort = firstHostPort(insp.NetworkSettings.Ports[mcPort()])
	rconHostPort = firstHostPort(insp.NetworkSettings.Ports[rconPort()])
	return mcHostPort, rconHostPort, nil
}

func firstHostPort(bindings []nat.PortBinding) int {
	if len(bindings) == 0 {
		return 0
	}
	p, _ := strconv.Atoi(bindings[0].HostPort)
	return p
}

// DieEvent describes a managed container that exited.
type DieEvent struct {
	ContainerID string
	ServerID    string // porque.server-id label, if present
	Name        string
	Role        string // porque.role label (e.g. "playit-agent")
	ExitCode    int
}

// WatchDieEvents streams "die" events for Porque-managed containers. The caller
// ranges over the returned channel until ctx is cancelled or an error arrives.
func (o *Orchestrator) WatchDieEvents(ctx context.Context) (<-chan DieEvent, <-chan error) {
	out := make(chan DieEvent)
	errc := make(chan error, 1)

	f := filters.NewArgs(
		filters.Arg("type", "container"),
		filters.Arg("event", "die"),
		filters.Arg("label", ManagedLabel+"=true"),
	)
	msgs, dErrs := o.cli.Events(ctx, events.ListOptions{Filters: f})

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-dErrs:
				if err != nil {
					select {
					case errc <- err:
					default:
					}
				}
				return
			case m := <-msgs:
				attrs := m.Actor.Attributes
				code, _ := strconv.Atoi(attrs["exitCode"])
				ev := DieEvent{
					ContainerID: m.Actor.ID,
					ServerID:    attrs["porque.server-id"],
					Name:        attrs["name"],
					Role:        attrs["porque.role"],
					ExitCode:    code,
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, errc
}

// statsJSON is a minimal projection of the Docker stats payload, decoded
// directly from the API to avoid SDK type churn.
type statsJSON struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs  uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
	} `json:"memory_stats"`
}

// ContainerStats samples a container twice to compute CPU percentage (relative
// to allocated cores) and current memory usage in bytes.
func (o *Orchestrator) ContainerStats(ctx context.Context, id string) (cpuPct float64, memBytes int64, err error) {
	resp, err := o.cli.ContainerStats(ctx, id, true)
	if err != nil {
		return 0, 0, fmt.Errorf("stats %s: %w", id, err)
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var s1, s2 statsJSON
	if err := dec.Decode(&s1); err != nil {
		return 0, 0, fmt.Errorf("stats decode: %w", err)
	}
	if err := dec.Decode(&s2); err != nil {
		return 0, 0, fmt.Errorf("stats decode: %w", err)
	}

	cpuDelta := float64(s2.CPUStats.CPUUsage.TotalUsage) - float64(s1.CPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s2.CPUStats.SystemUsage) - float64(s1.CPUStats.SystemUsage)
	ncpu := float64(s2.CPUStats.OnlineCPUs)
	if ncpu == 0 {
		ncpu = float64(len(s2.CPUStats.CPUUsage.PercpuUsage))
	}
	if ncpu == 0 {
		ncpu = 1
	}
	if sysDelta > 0 && cpuDelta > 0 {
		cpuPct = (cpuDelta / sysDelta) * ncpu * 100
	}
	return cpuPct, int64(s2.MemoryStats.Usage), nil
}

// ContainerHealth reports whether a container is running and its healthcheck
// status ("starting", "healthy", "unhealthy", or "none" when no healthcheck).
func (o *Orchestrator) ContainerHealth(ctx context.Context, id string) (running bool, health string, err error) {
	insp, err := o.cli.ContainerInspect(ctx, id)
	if err != nil {
		return false, "", fmt.Errorf("inspect %s: %w", id, err)
	}
	if insp.State == nil {
		return false, "none", nil
	}
	health = "none"
	if insp.State.Health != nil {
		health = insp.State.Health.Status
	}
	return insp.State.Running, health, nil
}

// FollowLogs returns a streaming reader of the container's combined stdout and
// stderr. The stream is multiplexed; callers must demux it (see stdcopy).
func (o *Orchestrator) FollowLogs(ctx context.Context, id string) (io.ReadCloser, error) {
	rc, err := o.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "200",
	})
	if err != nil {
		return nil, fmt.Errorf("logs %s: %w", id, err)
	}
	return rc, nil
}
