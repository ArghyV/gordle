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
// specified Docker image. The returned validator is ready for use; any Docker client
// creation errors are deferred until Validate is called.
func NewContainerValidator(image string) *ContainerValidator {
	return &ContainerValidator{image: image}
}

// Validate runs the given command inside a temporary container based on the validator's
// image. The command is executed with its working directory set to the read-only bind-mount
// of workdir at /work. Networking is disabled. The container is always removed after the
// command finishes, even on error paths.
//
// Parameters:
//   - cmd: the command line to execute inside the container.
//   - workdir: the host directory to bind-mount as /work (read-only). If empty, no mount is added.
//
// Returns:
//   - passed: true if the command exits with status 0, false otherwise.
//   - output: combined stdout and stderr of the command.
//   - err: any error encountered while setting up, running, or cleaning up the container.
func (v *ContainerValidator) Validate(cmd string, workdir string) (passed bool, output string, err error) {
	ctx := context.Background()

	// WithAPIVersionNegotiation must be called — it is a constructor, not a value.
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false, "", fmt.Errorf("validate: docker client creation failed: %w", err)
	}
	defer func() { _ = cli.Close() }()

	containerConfig := &container.Config{
		Image:      v.image,
		Cmd:        []string{"sh", "-c", cmd},
		WorkingDir: "/work",
	}
	hostConfig := &container.HostConfig{
		NetworkMode: "none",
		AutoRemove:  false,
	}
	if workdir != "" {
		hostConfig.Binds = []string{fmt.Sprintf("%s:/work:ro", workdir)}
	}

	// ContainerCreate now takes a single options struct instead of positional args.
	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     containerConfig,
		HostConfig: hostConfig,
	})
	if err != nil {
		return false, "", fmt.Errorf("validate: container creation failed: %w", err)
	}
	// ContainerRemove now returns (string, error); blank both in defer.
	defer func() {
		_, _ = cli.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		})
	}()

	// ContainerStart now returns (string, error); capture err, discard first return.
	if _, err = cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return false, "", fmt.Errorf("validate: container start failed: %w", err)
	}

	// ContainerWait now returns a single <-chan container.WaitResponse (no separate errCh).
	// The condition is passed via client.ContainerWaitOptions.
	waitCh := cli.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})
	result := <-waitCh
	if result.Error != nil {
		return false, "", fmt.Errorf("validate: container wait error: %s", result.Error.Message)
	}
	passed = result.StatusCode == 0

	// ContainerLogs options moved from types to container package.
	logReader, err := cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return false, "", fmt.Errorf("validate: retrieving container logs failed: %w", err)
	}
	defer logReader.Close()

	logBytes, err := io.ReadAll(logReader)
	if err != nil {
		return false, "", fmt.Errorf("validate: reading container logs failed: %w", err)
	}
	return passed, string(logBytes), nil
}
