package api

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/pkg/stdcopy"

	"github.com/Soif2Sang/imt-cloud-CI-CD-backend.git/internal/git"
	"github.com/Soif2Sang/imt-cloud-CI-CD-backend.git/internal/models"
	"github.com/Soif2Sang/imt-cloud-CI-CD-backend.git/internal/parser"
)

// runPipeline executes the CI/CD pipeline logic
// This unifies logic from webhook and manual trigger
func (s *Server) runPipelineLogic(params models.PipelineRunParams) {
	// Create a unique workspace directory
	workspaceDir := filepath.Join("/tmp", "cicd-workspaces", fmt.Sprintf("%s-%s-%d", params.RepoName, params.CommitHash[:8], time.Now().Unix()))

	log.Printf("Starting pipeline for %s", params.RepoName)

	// Clone the repository
	log.Printf("Cloning repository to %s", workspaceDir)
	if err := git.Clone(params.RepoURL, params.Branch, workspaceDir, params.AccessToken, params.CommitHash); err != nil {
		log.Printf("Failed to clone repository: %v", err)
		if s.db != nil && params.PipelineID > 0 {
			s.db.UpdatePipelineStatus(params.PipelineID, "failed")
		}
		return
	}
	defer git.Cleanup(workspaceDir)

	// Find and parse the CI config file
	configPath := filepath.Join(workspaceDir, params.PipelineFilename)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Printf("CI config file not found at %s", configPath)
		if s.db != nil && params.PipelineID > 0 {
			s.db.UpdatePipelineStatus(params.PipelineID, "failed")
		}
		return
	}

	log.Printf("Found CI config: %s", configPath)

	// Parse the CI config
	p := parser.NewParser(configPath)
	config, err := p.Parse()
	if err != nil {
		log.Printf("Failed to parse CI config: %v", err)
		if s.db != nil && params.PipelineID > 0 {
			s.db.UpdatePipelineStatus(params.PipelineID, "failed")
		}
		return
	}

	log.Printf("Config loaded with %d stages", len(config.Stages))

	// Execute the pipeline jobs
	pipelineSuccess := s.executePipeline(config, workspaceDir, params.PipelineID)

	// Deploy if successful
	if pipelineSuccess {
		log.Printf("Pipeline successful. Starting deployment using %s...", params.DeploymentFilename)

		var deploymentID int
		if s.db != nil && params.PipelineID > 0 {
			deploy, err := s.db.CreateDeployment(params.PipelineID)
			if err != nil {
				log.Printf("Failed to create deployment record: %v", err)
			} else {
				deploymentID = deploy.ID
			}
		}

		sanitizedRepoName := sanitizeProjectName(params.RepoName)
		logs, err := s.docker.DeployCompose(workspaceDir, params.DeploymentFilename, sanitizedRepoName)

		// Store logs
		if s.db != nil && params.PipelineID > 0 && logs != "" {
			if logErr := s.db.CreateDeploymentLog(params.PipelineID, logs); logErr != nil {
				log.Printf("Failed to store deployment logs: %v", logErr)
			}
		}

		if err != nil {
			log.Printf("Deployment failed: %v", err)
			pipelineSuccess = false
			if s.db != nil && deploymentID > 0 {
				s.db.UpdateDeploymentStatus(deploymentID, "failed")
			}
		} else {
			log.Printf("Deployment successful!")
			if s.db != nil && deploymentID > 0 {
				s.db.UpdateDeploymentStatus(deploymentID, "success")
			}
		}
	}

	// Update final pipeline status
	if s.db != nil && params.PipelineID > 0 {
		if pipelineSuccess {
			s.db.UpdatePipelineStatus(params.PipelineID, "success")
			log.Printf("Pipeline %d completed successfully", params.PipelineID)
		} else {
			s.db.UpdatePipelineStatus(params.PipelineID, "failed")
			log.Printf("Pipeline %d failed", params.PipelineID)
		}
	}
}

// executePipeline runs all jobs in the pipeline
func (s *Server) executePipeline(config *parser.PipelineConfig, workspaceDir string, pipelineID int) bool {
	pipelineSuccess := true

	for _, stageName := range config.Stages {
		log.Printf("Running stage: %s", stageName)

		for jobName, job := range config.Jobs {
			if job.Stage != stageName {
				continue
			}

			log.Printf("Running job: %s (image: %s)", jobName, job.Image)

			// Create job record in database
			var jobID int
			if s.db != nil && pipelineID > 0 {
				dbJob, err := s.db.CreateJob(pipelineID, jobName, job.Stage, job.Image)
				if err != nil {
					log.Printf("Failed to create job record: %v", err)
				} else {
					jobID = dbJob.ID
					s.db.UpdateJobStatus(jobID, "running", nil)
				}
			}

			// Pull the image
			log.Printf("Pulling image: %s", job.Image)
			if err := s.docker.PullImage(job.Image); err != nil {
				log.Printf("Failed to pull image %s: %v", job.Image, err)
				if s.db != nil && jobID > 0 {
					exitCode := 1
					s.db.UpdateJobStatus(jobID, "failed", &exitCode)
				}
				pipelineSuccess = false
				continue
			}

			// Run the job with workspace mounted
			containerID, err := s.docker.RunJobWithVolume(job.Image, job.Script, workspaceDir)
			if err != nil {
				log.Printf("Failed to start job %s: %v", jobName, err)
				if s.db != nil && jobID > 0 {
					exitCode := 1
					s.db.UpdateJobStatus(jobID, "failed", &exitCode)
				}
				pipelineSuccess = false
				continue
			}

			// Collect and store logs
			s.collectLogs(containerID, jobID)

			// Wait for container to finish
			statusCode, err := s.docker.WaitForContainer(containerID)
			if err != nil {
				log.Printf("Error waiting for container: %v", err)
			}

			// Update job status
			exitCode := int(statusCode)
			if s.db != nil && jobID > 0 {
				status := "success"
				if statusCode != 0 {
					status = "failed"
				}
				s.db.UpdateJobStatus(jobID, status, &exitCode)
			}

			if statusCode != 0 {
				log.Printf("Job %s failed with exit code %d", jobName, statusCode)
				pipelineSuccess = false
				// Stop pipeline on first failure
				return false
			}

			log.Printf("Job %s completed successfully", jobName)
		}
	}

	return pipelineSuccess
}

// collectLogs collects logs from the container and stores them in the database
func (s *Server) collectLogs(containerID string, jobID int) {
	reader, err := s.docker.GetLogs(containerID)
	if err != nil {
		log.Printf("Failed to get logs: %v", err)
		return
	}
	defer reader.Close()

	// Use a pipe to connect stdcopy (writer) to scanner (reader)
	pr, pw := io.Pipe()

	// Run stdcopy in a goroutine to demultiplex the docker stream
	go func() {
		// We write both stdout and stderr to the same pipe
		if _, err := stdcopy.StdCopy(pw, pw, reader); err != nil {
			log.Printf("Error demultiplexing logs: %v", err)
		}
		pw.Close()
	}()

	scanner := bufio.NewScanner(pr)
	var logBatch []string

	for scanner.Scan() {
		line := scanner.Text()

		// Sanitize line: remove null bytes (Postgres doesn't allow them in text)
		cleanLine := strings.ReplaceAll(line, "\x00", "")

		if cleanLine == "" {
			continue
		}

		// Print to console
		fmt.Println(cleanLine)

		// Add to batch
		logBatch = append(logBatch, cleanLine)

		// Store in batches of 10
		if len(logBatch) >= 10 && s.db != nil && jobID > 0 {
			if err := s.db.CreateLogBatch(jobID, logBatch); err != nil {
				log.Printf("Failed to store logs: %v", err)
			}
			logBatch = nil
		}
	}

	// Store remaining logs
	if len(logBatch) > 0 && s.db != nil && jobID > 0 {
		if err := s.db.CreateLogBatch(jobID, logBatch); err != nil {
			log.Printf("Failed to store remaining logs: %v", err)
		}
	}
}

// === Higher level Wrappers ===

// runPipelineFromWebhook adapts webhook data to the unified runner
func (s *Server) runPipelineFromWebhook(pushEvent models.PushEvent, branch, commitHash string) {
	// Find or create project in database
	var projectID int
	var accessToken string
	var pipelineFilename string
	var deploymentFilename string

	if s.db != nil {
		project, err := s.findOrCreateProject(pushEvent.Repository)
		if err != nil {
			log.Printf("Failed to find/create project: %v", err)
		} else {
			projectID = project.ID
			accessToken = project.AccessToken
			pipelineFilename = project.PipelineFilename
			deploymentFilename = project.DeploymentFilename
		}
	}

	if pipelineFilename == "" {
		pipelineFilename = ".gitlab-ci.yml"
	}
	if deploymentFilename == "" {
		deploymentFilename = "docker-compose.yml"
	}

	// Create pipeline record
	var pipelineID int
	if s.db != nil && projectID > 0 {
		pipeline, err := s.db.CreatePipeline(projectID, branch, commitHash)
		if err != nil {
			log.Printf("Failed to create pipeline record: %v", err)
		} else {
			pipelineID = pipeline.ID
			log.Printf("Pipeline created with ID: %d", pipelineID)
			s.db.UpdatePipelineStatus(pipelineID, "running")
		}
	}

	params := models.PipelineRunParams{
		RepoURL:            pushEvent.Repository.CloneURL,
		RepoName:           pushEvent.Repository.Name,
		Branch:             branch,
		CommitHash:         commitHash,
		AccessToken:        accessToken,
		PipelineFilename:   pipelineFilename,
		DeploymentFilename: deploymentFilename,
		ProjectID:          projectID,
		PipelineID:         pipelineID,
	}

	s.runPipelineLogic(params)
}

// runPipelineFromManualTrigger adapts manual trigger data to the unified runner
func (s *Server) runPipelineFromManualTrigger(project *models.Project, pipeline *models.Pipeline, branch string) {
	log.Printf("Starting manual pipeline %d for project %s", pipeline.ID, project.Name)

	// Update status to running
	s.db.UpdatePipelineStatus(pipeline.ID, "running")

	pipelineFilename := project.PipelineFilename
	if pipelineFilename == "" {
		pipelineFilename = ".gitlab-ci.yml"
	}
	deploymentFilename := project.DeploymentFilename
	if deploymentFilename == "" {
		deploymentFilename = "docker-compose.yml"
	}

	params := models.PipelineRunParams{
		RepoURL:            project.RepoURL,
		RepoName:           project.Name,
		Branch:             branch,
		CommitHash:         pipeline.CommitHash,
		AccessToken:        project.AccessToken,
		PipelineFilename:   pipelineFilename,
		DeploymentFilename: deploymentFilename,
		ProjectID:          project.ID,
		PipelineID:         pipeline.ID,
	}

	s.runPipelineLogic(params)
}
