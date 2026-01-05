package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Clone clones a repository to the destination path and checks out a specific commit
// If token is provided, it's used for authentication (HTTPS)
// If commitHash is provided, it checks out that specific commit after cloning
func Clone(repoURL, branch, destPath, token, commitHash string) error {
	// If token provided, inject it into the URL for auth
	// https://github.com/user/repo.git -> https://token@github.com/user/repo.git
	if token != "" {
		repoURL = injectToken(repoURL, token)
	}

	// If we need a specific commit, we can't use shallow clone
	// because the commit might not be the latest on the branch
	var args []string
	if commitHash != "" {
		// Full clone to ensure we have the commit
		args = []string{"clone", "--branch", branch, repoURL, destPath}
	} else {
		// Shallow clone if no specific commit needed
		args = []string{"clone", "--depth", "1", "--branch", branch, repoURL, destPath}
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s - %w", string(output), err)
	}

	// Checkout specific commit if provided
	if commitHash != "" {
		if err := Checkout(destPath, commitHash); err != nil {
			return fmt.Errorf("failed to checkout commit %s: %w", commitHash, err)
		}
	}

	return nil
}

// Checkout checks out a specific commit in the repository
func Checkout(repoPath, commitHash string) error {
	cmd := exec.Command("git", "checkout", commitHash)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout failed: %s - %w", string(output), err)
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