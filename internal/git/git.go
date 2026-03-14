package git

import (
	"os/exec"
	"strings"
)

// GetCommitsSinceLastTag returns a list of commit messages since the last tag
func GetCommitsSinceLastTag() ([]string, error) {
	// 1. Get the last tag
	tagCmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	tagBytes, err := tagCmd.Output()
	
	var lastTag string
	if err != nil {
		// If no tag found, get first commit
		firstCommitCmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
		firstCommitBytes, err := firstCommitCmd.Output()
		if err != nil {
			return nil, err
		}
		lastTag = strings.TrimSpace(string(firstCommitBytes))
	} else {
		lastTag = strings.TrimSpace(string(tagBytes))
	}

	// 2. Get logs from lastTag to HEAD
	logCmd := exec.Command("git", "log", lastTag+"..HEAD", "--oneline")
	logBytes, err := logCmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	var result []string
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}

	return result, nil
}
