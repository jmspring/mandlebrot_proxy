package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type DockerManager struct {
	cli      *client.Client
	img      string
	hostPort int
}

func NewDockerManager(img string, hostPort int) (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &DockerManager{cli: cli, img: img, hostPort: hostPort}, nil
}

func (dm *DockerManager) Start(ctx context.Context) (string, error) {
	slog.Info("pulling image", "image", dm.img)
	rc, err := dm.cli.ImagePull(ctx, dm.img, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("pull %s: %w", dm.img, err)
	}
	// Pull isn't done until the reader is drained.
	io.Copy(io.Discard, rc)
	rc.Close()

	p := nat.Port("80/tcp")
	resp, err := dm.cli.ContainerCreate(ctx,
		&container.Config{
			Image:        dm.img,
			ExposedPorts: nat.PortSet{p: {}},
		},
		&container.HostConfig{
			PortBindings: nat.PortMap{
				p: {{HostIP: "127.0.0.1", HostPort: fmt.Sprintf("%d", dm.hostPort)}},
			},
		},
		nil, nil, "mandelbrot-auth-proxy",
	)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if err := dm.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}
	return resp.ID, nil
}

func (dm *DockerManager) Stop(ctx context.Context, id string) error {
	timeout := 10
	dm.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})

	if err := dm.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}
	slog.Info("container removed", "id", id[:12])
	return nil
}

func (dm *DockerManager) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.After(timeout)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	addr := fmt.Sprintf("http://127.0.0.1:%d/", dm.hostPort)
	hc := &http.Client{Timeout: 2 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("container not ready after %v", timeout)
		case <-tick.C:
			resp, err := hc.Get(addr)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}
