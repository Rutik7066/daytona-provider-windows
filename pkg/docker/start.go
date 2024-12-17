// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bufio"
	"errors"
	"io"
	"strings"

	"github.com/daytonaio/daytona/pkg/provider/util"
	"github.com/daytonaio/daytona/pkg/workspace/project"
	"github.com/docker/docker/api/types/container"
)

func (d *DockerClient) StartProject(opts *CreateProjectOptions, daytonaDownloadUrl string) error {
	var err error
	containerUser := opts.Project.User

	err = d.startImageProject(opts)
	if err != nil {
		return err
	}

	return d.startDaytonaAgent(opts.Project, containerUser, daytonaDownloadUrl, opts.LogWriter)
}

func (d *DockerClient) startDaytonaAgent(p *project.Project, containerUser, daytonaDownloadUrl string, logWriter io.Writer) error {
	errChan := make(chan error)

	r, w := io.Pipe()
	writer := io.MultiWriter(w, logWriter)

	go func() {
		result, err := d.ExecSync(d.GetProjectContainerName(p), container.ExecOptions{
			Cmd:          []string{"bash", "-c", util.GetProjectStartScript(daytonaDownloadUrl, p.ApiKey)},
			AttachStdout: true,
			AttachStderr: true,
			User:         containerUser,
		}, writer)
		if err != nil {
			errChan <- err
			return
		}

		if result.ExitCode != 0 {
			errChan <- errors.New(result.StdErr)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "Daytona Agent started") {
				errChan <- nil
				return
			}
		}
	}()

	return <-errChan
}
