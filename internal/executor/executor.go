package executor

import (
	"context"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

type DockerExecutor struct {
	cli *client.Client
	ctx context.Context
}

func NewDockerExecutor() (*DockerExecutor, error) {
	// Initialise le client en utilisant les variables d'environnement locales
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerExecutor{
		cli: cli,
		ctx: context.Background(),
	}, nil
}

func (e *DockerExecutor) PullImage(imageName string) error {
	reader, err := e.cli.ImagePull(e.ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	// On lit le flux jusqu'au bout pour attendre la fin du pull
	_, err = io.Copy(io.Discard, reader)
	return err
}

func (e *DockerExecutor) RunJob(imageName string, commands []string) (string, error) {
	// On concatène les commandes avec " && " pour qu'elles s'exécutent séquentiellement
	cmdString := strings.Join(commands, " && ")

	// 1. Configurer le conteneur avec vos commandes
	resp, err := e.cli.ContainerCreate(e.ctx, &container.Config{
		Image:      imageName,
		Cmd:        []string{"sh", "-c", cmdString}, // On encapsule dans un shell
		WorkingDir: "/workspace",
	}, nil, nil, nil, "")
	if err != nil {
		return "", err
	}

	// 2. Démarrer le conteneur
	err = e.cli.ContainerStart(e.ctx, resp.ID, container.StartOptions{})
	return resp.ID, err
}

// RunJobWithVolume runs a job with a workspace directory mounted into the container
func (e *DockerExecutor) RunJobWithVolume(imageName string, commands []string, workspacePath string) (string, error) {
	// On concatène les commandes avec " && " pour qu'elles s'exécutent séquentiellement
	cmdString := strings.Join(commands, " && ")

	// Configuration du conteneur
	containerConfig := &container.Config{
		Image:      imageName,
		Cmd:        []string{"sh", "-c", cmdString},
		WorkingDir: "/workspace", // Le répertoire de travail dans le conteneur
	}

	// Configuration de l'hôte avec le volume monté
	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: workspacePath,        // Chemin sur l'hôte
				Target: "/workspace",         // Chemin dans le conteneur
			},
		},
	}

	// Créer le conteneur
	resp, err := e.cli.ContainerCreate(e.ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return "", err
	}

	// Démarrer le conteneur
	err = e.cli.ContainerStart(e.ctx, resp.ID, container.StartOptions{})
	return resp.ID, err
}

func (e *DockerExecutor) GetLogs(containerID string) (io.ReadCloser, error) {
	return e.cli.ContainerLogs(e.ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true, // Important pour le temps réel
	})
}

func (e *DockerExecutor) WaitForContainer(containerID string) (int64, error) {
	statusCh, errCh := e.cli.ContainerWait(e.ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return 0, err
	case status := <-statusCh:
		return status.StatusCode, nil
	}
}

// RemoveContainer removes a container (cleanup)
func (e *DockerExecutor) RemoveContainer(containerID string) error {
	return e.cli.ContainerRemove(e.ctx, containerID, container.RemoveOptions{
		Force: true,
	})
}