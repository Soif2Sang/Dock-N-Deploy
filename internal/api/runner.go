package api

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Soif2Sang/imt-cloud-CI-CD-backend.git/internal/git"
	"github.com/Soif2Sang/imt-cloud-CI-CD-backend.git/internal/models"
	"github.com/Soif2Sang/imt-cloud-CI-CD-backend.git/internal/parser/pipeline"
	"github.com/Soif2Sang/imt-cloud-CI-CD-backend.git/pkg/logger"
	"github.com/Soif2Sang/imt-cloud-CI-CD-backend.git/pkg/utils"
)

// runPipelineLogic executes the CI/CD pipeline logic
func (s *Server) runPipelineLogic(params models.PipelineRunParams) {
	project, _ := s.db.GetProject(params.ProjectID)

	workspaceDir := filepath.Join(os.TempDir(), "cicd-workspaces", fmt.Sprintf("%s-%s-%d", utils.SanitizeProjectName(params.RepoName), params.CommitHash[:8], time.Now().Unix()))

	logger.Info(fmt.Sprintf("Starting pipeline for %s", params.RepoName))
	logger.Info(fmt.Sprintf("Cloning repository to %s", workspaceDir))

	if err := git.Clone(params.RepoURL, params.Branch, workspaceDir, params.AccessToken, params.CommitHash); err != nil {
		logger.Error("Failed to clone repository: " + err.Error())
		s.db.UpdatePipelineStatus(params.PipelineID, "failed")
		return
	}

	defer git.Cleanup(workspaceDir)

	// Find and parse the CI config file
	configPath := filepath.Join(workspaceDir, params.PipelineFilename)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logger.Warn(fmt.Sprintf("CI config file not found at %s", configPath))
		s.db.UpdatePipelineStatus(params.PipelineID, "failed")
		return
	}

	logger.Info(fmt.Sprintf("Found CI config: %s", configPath))

	// Parse the CI config
	p := pipeline.NewParser(configPath)
	config, err := p.Parse()
	if err != nil {
		logger.Error("Failed to parse CI config: " + err.Error())
		s.db.UpdatePipelineStatus(params.PipelineID, "failed")
		return
	}

	logger.Info(fmt.Sprintf("Config loaded with %d stages", len(config.Stages)))

	// Pre-create jobs
	for _, stageName := range config.Stages {
		for jobName, job := range config.Jobs {
			if job.Stage == stageName {
				if _, err := s.db.CreateJob(params.PipelineID, jobName, job.Stage, job.Image); err != nil {
					logger.Error(fmt.Sprintf("Failed to pre-create job %s: %v", jobName, err))
					return
				}
			}
		}
	}

	// Pre-create deployment
	deployment, err := s.db.CreatePendingDeployment(params.PipelineID)
	if err != nil {
		logger.Error("Failed to pre-create deployment: " + err.Error())
		return
	}

	// Execute the pipeline jobs using delegated executor
	pipelineSuccess := s.pipelineExecutor.Execute(config, workspaceDir, params.PipelineID, project)

	// Deploy if successful
	if pipelineSuccess {
		logger.Info(fmt.Sprintf("Pipeline successful. Starting deployment using %s...", params.DeploymentFilename))

		s.db.UpdateDeploymentStatus(deployment.ID, "deploying")

		// Deploy to environment using delegated executor
		_, err := s.deploymentExecutor.Execute(project, params, workspaceDir, deployment.ID)

		if err != nil {
			logger.Error("Deployment failed: " + err.Error())

			// Attempt Rollback
			rollbackSuccess := false
			lastPipeline, _ := s.db.GetLastSuccessfulPipeline(project.ID)

			if lastPipeline == nil {
				logger.Info("No previous successful pipeline found; skipping rollback")
			} else {
				logger.Info(fmt.Sprintf("Attempting rollback to commit %s", lastPipeline.CommitHash))

				// Prepare rollback params
				rollbackParams := params
				rollbackParams.CommitHash = lastPipeline.CommitHash
				// Note: We use the same config filenames as current project settings.

				// Create unique workspace for rollback (use OS temp dir)
				rollbackDir := filepath.Join(os.TempDir(), "cicd-workspaces", fmt.Sprintf("%s-rollback-%s-%d", utils.SanitizeProjectName(params.RepoName), rollbackParams.CommitHash[:8], time.Now().Unix()))

				logger.Info(fmt.Sprintf("Cloning rollback commit to %s", rollbackDir))
				if cloneErr := git.Clone(rollbackParams.RepoURL, rollbackParams.Branch, rollbackDir, rollbackParams.AccessToken, rollbackParams.CommitHash); cloneErr == nil {
					defer git.Cleanup(rollbackDir)

					// Log rollback start
					s.db.CreateDeploymentLog(deployment.ID, "=== ROLLBACK STARTED ===")

					// Run deployment for old version using delegated executor
					_, rbErr := s.deploymentExecutor.Execute(project, rollbackParams, rollbackDir, deployment.ID)

					if rbErr == nil {
						rollbackSuccess = true
						logger.Info("Rollback successful")
					} else {
						logger.Error("Rollback failed: " + rbErr.Error())
					}
				} else {
					logger.Error("Rollback clone failed: " + cloneErr.Error())
				}
			}

			pipelineSuccess = false
			if rollbackSuccess {
				s.db.UpdateDeploymentStatus(deployment.ID, "rolled_back")
			} else {
				s.db.UpdateDeploymentStatus(deployment.ID, "failed")
			}

		} else {
			logger.Info("Deployment successful!")
			s.db.UpdateDeploymentStatus(deployment.ID, "success")
		}
	}

	// Update final pipeline status
	if pipelineSuccess {
		logger.Info(fmt.Sprintf("Pipeline %d completed successfully", params.PipelineID))
		s.db.UpdatePipelineStatus(params.PipelineID, "success")
	} else {
		logger.Error(fmt.Sprintf("Pipeline %d failed", params.PipelineID))
		s.db.UpdatePipelineStatus(params.PipelineID, "failed")
		s.db.UpdateDeploymentStatus(deployment.ID, "failed")
	}
}
