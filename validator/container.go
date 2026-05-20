// Package validator provides a container‑based implementation of the Validator interface.
// It does not implement any other functionality of the gordle system.
package validator

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// ContainerValidator runs validation commands inside an isolated Docker container.
type ContainerValidator struct {
	// image is the Docker image used for the container.
	image string
}

// NewContainerValidator creates a new ContainerValidator that will run commands in the
// specified Docker image. The returned validator is ready for use; any Docker client
// creation errors are deferred until Validate is called.
func NewContainerValidator(image string) *ContainerValidator {
	return &ContainerValidator{
		image: image,
	}
}

// Validate runs the given command inside a temporary container based on the validator's
// image. The command is executed with its working directory set to the read‑only bind‑mount
// of workdir at /work. Networking is disabled. The container is always removed after the
// command finishes, even on error paths.
//
// Parameters:
//   - cmd: the command line to execute inside the container.
//   - workdir: the host directory to bind‑mount as /work (read‑only). If empty, no mount is added.
//
// Returns:
//   - passed: true if the command exits with status 0, false otherwise.
//   - output: combined stdout and stderr of the command.
//   - err: any error encountered while setting up, running, or cleaning up the container.
func (v *ContainerValidator) Validate(cmd string, workdir string) (passed bool, output string, err error) {
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation)
	if err != nil {
		return false, "", fmt.Errorf("validate: docker client creation failed: %w", err)
	}
	// Ensure the client connection is closed.
	defer func() {
		_ = cli.Close()
	}()

	// Container configuration.
	containerConfig := &container.Config{
		Image:      v.image,
		Cmd:        []string{"sh", "-c", cmd},
		WorkingDir: "/work",
	}
	// Host configuration: read‑only bind‑mount and disabled networking.
	hostConfig := &container.HostConfig{
		NetworkMode: "none",
		AutoRemove:  false,
	}
	if workdir != "" {
		hostConfig.Binds = []string{fmt.Sprintf("%s:/work:ro", workdir)}
	}

	// Create the container.
	resp, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return false, "", fmt.Errorf("validate: container creation failed: %w", err)
	}
	// Ensure the container is removed after execution.
	defer func() {
		_ = cli.ContainerRemove(context.Background(), resp.ID, types.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		})
	}()

	// Start the container.
	if err = cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return false, "", fmt.Errorf("validate: container start failed: %w", err)
	}

	// Wait for the container to finish.
	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err = <-errCh:
		if err != nil {
			return false, "", fmt.Errorf("validate: container wait error: %w", err)
		}
	case status := <-statusCh:
		passed = status.StatusCode == 0
		// Retrieve logs regardless of exit status.
		var logReader io.ReadCloser
		logReader, err = cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
		})
		if err != nil {
			return false, "", fmt.Errorf("validate: retrieving container logs failed: %w", err)
		}
		defer logReader.Close()
		var logBytes []byte
		logBytes, err = io.ReadAll(logReader)
		if err != nil {
			return false, "", fmt.Errorf("validate: reading container logs failed: %w", err)
		}
		output = string(logBytes)
		return passed, output, nil
	}
	// Should not reach here.
	return false, "", fmt.Errorf("validate: unexpected wait condition")
}
