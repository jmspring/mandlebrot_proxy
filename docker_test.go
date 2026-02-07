package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/client"
)

func dockerAvailable() bool {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer cli.Close()
	_, err = cli.Ping(context.Background())
	return err == nil
}

func TestDockerLifecycle_Live(t *testing.T) {
	if os.Getenv("TEST_DOCKER") == "" && !dockerAvailable() {
		t.Skip("no docker daemon; set TEST_DOCKER=1 to force")
	}

	// Use a high port to avoid conflicts with a running instance.
	port := 18080
	dm, err := NewDockerManager("lechgu/mandelbrot", port)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cid, err := dm.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("started container %s", cid[:12])

	// always clean up
	defer func() {
		if err := dm.Stop(context.Background(), cid); err != nil {
			t.Errorf("stop: %v", err)
		}
	}()

	if err := dm.WaitReady(ctx, 30*time.Second); err != nil {
		t.Fatal(err)
	}

	// sanity: the container should respond
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestWaitReady_CancelledContext(t *testing.T) {
	dm := &DockerManager{hostPort: 19999} // port nobody is listening on
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := dm.WaitReady(ctx, 5*time.Second)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}
