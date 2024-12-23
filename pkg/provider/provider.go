package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"

	internal "github.com/Rutik7066/daytona-provider-windows/internal"
	log_writers "github.com/Rutik7066/daytona-provider-windows/internal/log"
	"github.com/Rutik7066/daytona-provider-windows/pkg/client"
	provider_types "github.com/Rutik7066/daytona-provider-windows/pkg/types"

	"github.com/Rutik7066/daytona-provider-windows/pkg/docker"
	"github.com/daytonaio/daytona/pkg/build/detect"
	"github.com/daytonaio/daytona/pkg/logs"
	"github.com/daytonaio/daytona/pkg/provider"
	provider_util "github.com/daytonaio/daytona/pkg/provider/util"
	"github.com/daytonaio/daytona/pkg/ssh"
	"github.com/daytonaio/daytona/pkg/workspace"
	"github.com/daytonaio/daytona/pkg/workspace/project"
	docker_sdk "github.com/docker/docker/client"
)

type WindowsProvider struct {
	BasePath           *string
	DaytonaDownloadUrl *string
	DaytonaVersion     *string
	ServerUrl          *string
	ApiUrl             *string
	LogsDir            *string
	ApiPort            *uint32
	ServerPort         *uint32
	RemoteSockDir      string
}

func (p *WindowsProvider) Initialize(req provider.InitializeProviderRequest) (*provider_util.Empty, error) {
	tmpDir := "/tmp"
	if runtime.GOOS == "windows" {
		tmpDir = os.TempDir()
		if tmpDir == "" {
			return new(provider_util.Empty), errors.New("could not determine temp dir")
		}
	}

	p.RemoteSockDir = path.Join(tmpDir, "target-socks")

	// Clear old sockets
	err := os.RemoveAll(p.RemoteSockDir)
	if err != nil {
		return new(provider_util.Empty), err
	}
	err = os.MkdirAll(p.RemoteSockDir, 0755)
	if err != nil {
		return new(provider_util.Empty), err
	}

	p.BasePath = &req.BasePath
	p.DaytonaDownloadUrl = &req.DaytonaDownloadUrl
	p.DaytonaVersion = &req.DaytonaVersion
	p.ServerUrl = &req.ServerUrl
	p.ApiUrl = &req.ApiUrl
	p.LogsDir = &req.LogsDir
	p.ApiPort = &req.ApiPort
	p.ServerPort = &req.ServerPort

	return new(provider_util.Empty), nil
}

func (p WindowsProvider) GetInfo() (provider.ProviderInfo, error) {
	label := "Windows"

	return provider.ProviderInfo{
		Name:    "windows-provider",
		Label:   &label,
		Version: internal.Version,
	}, nil
}

func (p WindowsProvider) GetTargetManifest() (*provider.ProviderTargetManifest, error) {
	return provider_types.GetTargetManifest(), nil
}

func (p WindowsProvider) GetPresetTargets() (*[]provider.ProviderTarget, error) {
	info, err := p.GetInfo()
	if err != nil {
		return nil, err
	}

	presetTargets := []provider.ProviderTarget{
		{
			Name:         "local",
			ProviderInfo: info,
			Options:      "{\n\t\"Sock Path\": \"/var/run/docker.sock\"\n}",
		},
	}
	return &presetTargets, nil
}

func (p WindowsProvider) StartWorkspace(workspaceReq *provider.WorkspaceRequest) (*provider_util.Empty, error) {
	return new(provider_util.Empty), nil
}

func (p WindowsProvider) StopWorkspace(workspaceReq *provider.WorkspaceRequest) (*provider_util.Empty, error) {
	return new(provider_util.Empty), nil
}

func (p WindowsProvider) DestroyWorkspace(workspaceReq *provider.WorkspaceRequest) (*provider_util.Empty, error) {
	dockerClient, err := p.getClient(workspaceReq.TargetOptions)
	if err != nil {
		return new(provider_util.Empty), err
	}

	workspaceDir, err := p.getWorkspaceDir(workspaceReq)
	if err != nil {
		return new(provider_util.Empty), err
	}

	sshClient, err := p.getSshClient(workspaceReq.Workspace.Target, workspaceReq.TargetOptions)
	if err != nil {
		return new(provider_util.Empty), err
	}
	if sshClient != nil {
		defer sshClient.Close()
	}

	err = dockerClient.DestroyWorkspace(workspaceReq.Workspace, workspaceDir, sshClient)
	if err != nil {
		return new(provider_util.Empty), err
	}

	return new(provider_util.Empty), nil
}

func (p WindowsProvider) GetWorkspaceInfo(workspaceReq *provider.WorkspaceRequest) (*workspace.WorkspaceInfo, error) {
	dockerClient, err := p.getClient(workspaceReq.TargetOptions)
	if err != nil {
		return nil, err
	}

	return dockerClient.GetWorkspaceInfo(workspaceReq.Workspace)
}

func (p WindowsProvider) StartProject(projectReq *provider.ProjectRequest) (*provider_util.Empty, error) {
	dockerClient, err := p.getClient(projectReq.TargetOptions)
	if err != nil {
		return new(provider_util.Empty), err
	}

	projectDir, err := p.getProjectDir(projectReq)
	if err != nil {
		return new(provider_util.Empty), err
	}

	logWriter := io.MultiWriter(&log_writers.InfoLogWriter{})
	if p.LogsDir != nil {
		loggerFactory := logs.NewLoggerFactory(p.LogsDir, nil)
		projectLogWriter := loggerFactory.CreateProjectLogger(projectReq.Project.WorkspaceId, projectReq.Project.Name, logs.LogSourceProvider)
		logWriter = io.MultiWriter(&log_writers.InfoLogWriter{}, projectLogWriter)
		defer projectLogWriter.Close()
	}

	downloadUrl := *p.DaytonaDownloadUrl
	var sshClient *ssh.Client

	if projectReq.Project.Target == "local" {
		builderType, err := detect.DetectProjectBuilderType(projectReq.Project.BuildConfig, projectDir, nil)
		if err != nil {
			return new(provider_util.Empty), err
		}

		if builderType != detect.BuilderTypeDevcontainer {
			parsed, err := url.Parse(downloadUrl)
			if err != nil {
				return new(provider_util.Empty), err
			}

			parsed.Host = fmt.Sprintf("host.docker.internal:%d", *p.ApiPort)
			parsed.Scheme = "http"
			downloadUrl = parsed.String()
		}
	} else {
		sshClient, err = p.getSshClient(projectReq.Project.Target, projectReq.TargetOptions)
		if err != nil {
			return new(provider_util.Empty), err
		}
		defer sshClient.Close()
	}

	err = dockerClient.StartProject(&docker.CreateProjectOptions{
		Project:                  projectReq.Project,
		ProjectDir:               projectDir,
		ContainerRegistry:        projectReq.ContainerRegistry,
		BuilderImage:             "rutik7066/daytona-windows-container",
		BuilderContainerRegistry: projectReq.BuilderContainerRegistry,
		LogWriter:                logWriter,
		Gpc:                      projectReq.GitProviderConfig,
		SshClient:                sshClient,
	}, downloadUrl)
	if err != nil {
		return new(provider_util.Empty), err
	}

	go func() {
		err = dockerClient.GetContainerLogs(dockerClient.GetProjectContainerName(projectReq.Project), logWriter)
		if err != nil {
			logWriter.Write([]byte(err.Error()))
		}
	}()

	return new(provider_util.Empty), nil
}

func (p WindowsProvider) StopProject(projectReq *provider.ProjectRequest) (*provider_util.Empty, error) {
	dockerClient, err := p.getClient(projectReq.TargetOptions)
	if err != nil {
		return new(provider_util.Empty), err
	}

	logWriter := io.MultiWriter(&log_writers.InfoLogWriter{})
	if p.LogsDir != nil {
		loggerFactory := logs.NewLoggerFactory(p.LogsDir, nil)
		projectLogWriter := loggerFactory.CreateProjectLogger(projectReq.Project.WorkspaceId, projectReq.Project.Name, logs.LogSourceProvider)
		logWriter = io.MultiWriter(&log_writers.InfoLogWriter{}, projectLogWriter)
		defer projectLogWriter.Close()
	}

	return new(provider_util.Empty), dockerClient.StopProject(projectReq.Project, logWriter)
}

func (p WindowsProvider) DestroyProject(projectReq *provider.ProjectRequest) (*provider_util.Empty, error) {
	dockerClient, err := p.getClient(projectReq.TargetOptions)
	if err != nil {
		return new(provider_util.Empty), err
	}

	projectDir, err := p.getProjectDir(projectReq)
	if err != nil {
		return new(provider_util.Empty), err
	}

	sshClient, err := p.getSshClient(projectReq.Project.Target, projectReq.TargetOptions)
	if err != nil {
		return new(provider_util.Empty), err
	}
	if sshClient != nil {
		defer sshClient.Close()
	}

	err = dockerClient.DestroyProject(projectReq.Project, projectDir, sshClient)
	if err != nil {
		return new(provider_util.Empty), err
	}

	return new(provider_util.Empty), nil
}

func (p WindowsProvider) GetProjectInfo(projectReq *provider.ProjectRequest) (*project.ProjectInfo, error) {
	dockerClient, err := p.getClient(projectReq.TargetOptions)
	if err != nil {
		return nil, err
	}

	return dockerClient.GetProjectInfo(projectReq.Project)
}

func (p WindowsProvider) getClient(targetOptionsJson string) (docker.IDockerClient, error) {
	targetOptions, err := provider_types.ParseTargetOptions(targetOptionsJson)
	if err != nil {
		return nil, err
	}

	client, err := client.GetClient(*targetOptions, p.RemoteSockDir)
	if err != nil {
		return nil, err
	}

	return docker.NewDockerClient(docker.DockerClientConfig{
		ApiClient: client,
	}), nil
}

func (p WindowsProvider) CheckRequirements() (*[]provider.RequirementStatus, error) {
	var results []provider.RequirementStatus
	ctx := context.Background()

	cli, err := docker_sdk.NewClientWithOpts(docker_sdk.FromEnv, docker_sdk.WithAPIVersionNegotiation())
	if err != nil {
		results = append(results, provider.RequirementStatus{
			Name:   "Docker installed",
			Met:    false,
			Reason: "Docker is not installed",
		})
		return &results, nil
	} else {
		results = append(results, provider.RequirementStatus{
			Name:   "Docker installed",
			Met:    true,
			Reason: "Docker is installed",
		})
	}

	// Check if Docker is running by fetching Docker info
	_, err = cli.Info(ctx)
	if err != nil {
		results = append(results, provider.RequirementStatus{
			Name:   "Docker running",
			Met:    false,
			Reason: "Docker is not running. Error: " + err.Error(),
		})
	} else {
		results = append(results, provider.RequirementStatus{
			Name:   "Docker running",
			Met:    true,
			Reason: "Docker is running",
		})
	}
	return &results, nil
}

// If the project is running locally, we override the env vars to use the host.docker.internal address
func (p WindowsProvider) setLocalEnvOverride(project *project.Project) {
	project.EnvVars["DAYTONA_SERVER_URL"] = fmt.Sprintf("http://host.docker.internal:%d", *p.ServerPort)
	project.EnvVars["DAYTONA_SERVER_API_URL"] = fmt.Sprintf("http://host.docker.internal:%d", *p.ApiPort)
}

func (p *WindowsProvider) getProjectDir(projectReq *provider.ProjectRequest) (string, error) {
	if projectReq.Project.Target == "local" {
		return filepath.Join(*p.BasePath, projectReq.Project.WorkspaceId, fmt.Sprintf("%s-%s", projectReq.Project.WorkspaceId, projectReq.Project.Name)), nil
	}

	targetOptions, err := provider_types.ParseTargetOptions(projectReq.TargetOptions)
	if err != nil {
		return "", err
	}

	// Using path instead of filepath because we always want to use / as the separator
	return path.Join(*targetOptions.WorkspaceDataDir, projectReq.Project.WorkspaceId, fmt.Sprintf("%s-%s", projectReq.Project.WorkspaceId, projectReq.Project.Name)), nil
}

func (p *WindowsProvider) getWorkspaceDir(workspaceReq *provider.WorkspaceRequest) (string, error) {
	if workspaceReq.Workspace.Target == "local" {
		return filepath.Join(*p.BasePath, workspaceReq.Workspace.Id), nil
	}

	targetOptions, err := provider_types.ParseTargetOptions(workspaceReq.TargetOptions)
	if err != nil {
		return "", err
	}

	// Using path instead of filepath because we always want to use / as the separator
	return path.Join(*targetOptions.WorkspaceDataDir, workspaceReq.Workspace.Id), nil
}

func (p *WindowsProvider) getSshClient(targetName string, targetOptionsJson string) (*ssh.Client, error) {
	if targetName == "local" {
		return nil, nil
	}

	targetOptions, err := provider_types.ParseTargetOptions(targetOptionsJson)
	if err != nil {
		return nil, err
	}

	return ssh.NewClient(&ssh.SessionConfig{
		Hostname:       *targetOptions.RemoteHostname,
		Port:           *targetOptions.RemotePort,
		Username:       *targetOptions.RemoteUser,
		Password:       targetOptions.RemotePassword,
		PrivateKeyPath: targetOptions.RemotePrivateKey,
	})
}
