package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Clone clones a repository to the destination path
// If token is provided, it's used for authentication (HTTPS)
func Clone(repoURL, branch, destPath, token string) error {
	// If token provided, inject it into the URL for auth
	// https://github.com/user/repo.git -> https://token@github.com/user/repo.git
	if token != "" {
		repoURL = injectToken(repoURL, token)
	}

	args := []string{"clone", "--depth", "1", "--branch", branch, repoURL, destPath}

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s - %w", string(output), err)
	}

	return nil
}

// Cleanup removes the cloned repository directory
func Cleanup(destPath string) error {
	return os.RemoveAll(destPath)
}

// injectToken adds token to HTTPS URL for authentication
func injectToken(repoURL, token string) string {
	// https://github.com/user/repo.git -> https://TOKEN@github.com/user/repo.git
	if strings.HasPrefix(repoURL, "https://") {
		return strings.Replace(repoURL, "https://", "https://"+token+"@", 1)
	}
	return repoURL
}

// GetLatestCommitHash returns the HEAD commit hash (optional but useful)
func GetLatestCommitHash(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
