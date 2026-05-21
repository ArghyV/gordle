// Package validator provides a container-based implementation of the Validator interface.
// It does not implement any other functionality of the gordle system.
package validator

import (
	"context"
	"fmt"
	"io"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// ContainerValidator runs validation commands inside an isolated Docker container.
type ContainerValidator struct {
	image string
}

// NewContainerValidator creates a new ContainerValidator that will run commands in the
// specified Docker image. Docker client errors are deferred until Validate is called.
func NewContainerValidator(image string) *ContainerValidator {
	return &ContainerValidator{image: image}
}

// Validate runs cmd inside a temporary container bind-mounting workdir at /work (read-only).
// Networking is disabled. The container is always destroyed after the run, including error paths.
//
// Returns passed=true if cmd exits 0, output=combined stdout+stderr, err on setup/teardown failure.
func (v *ContainerValidator) Validate(cmd string, workdir string) (passed bool, output string, err error) {
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false, "", fmt.Errorf("validate: docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	hostConfig := &container.HostConfig{
		NetworkMode: "none",
		AutoRemove:  false,
	}
	if workdir != "" {
		hostConfig.Binds = []string{fmt.Sprintf("%s:/work:ro", workdir)}
	}

	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      v.image,
			Cmd:        []string{"sh", "-c", cmd},
			WorkingDir: "/work",
		},
		HostConfig: hostConfig,
	})
	if err != nil {
		return false, "", fmt.Errorf("validate: container create: %w", err)
	}
	defer func() {
		_, _ = cli.ContainerRemove(context.Background(), resp.ID, client.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		})
	}()

	if _, err = cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		return false, "", fmt.Errorf("validate: container start: %w", err)
	}

	// ContainerWait returns a struct with Result and Error channels; it blocks until acknowledged.
	waitResult := cli.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})
	select {
	case err = <-waitResult.Error:
		return false, "", fmt.Errorf("validate: container wait: %w", err)
	case status := <-waitResult.Result:
		passed = status.StatusCode == 0
	}

	// ContainerLogsResult implements io.ReadCloser directly.
	logRC, err := cli.ContainerLogs(ctx, resp.ID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return false, "", fmt.Errorf("validate: container logs: %w", err)
	}
	defer logRC.Close()

	logBytes, err := io.ReadAll(logRC)
	if err != nil {
		return false, "", fmt.Errorf("validate: read logs: %w", err)
	}
	return passed, string(logBytes), nil
}
