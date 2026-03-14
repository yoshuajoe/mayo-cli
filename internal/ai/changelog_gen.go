package ai

import (
	"context"
	"fmt"
	"strings"
)

// GenerateChangelogFromCommits uses AI to summarize git commits into a professional changelog
func GenerateChangelogFromCommits(ctx context.Context, ai AIClient, commits []string, version string) (string, error) {
	if len(commits) == 0 {
		return fmt.Sprintf("# v%s\n\n- No changes documented.", version), nil
	}

	commitList := strings.Join(commits, "\n")
	
	systemPrompt := `You are a Senior Technical Writer. 
Your task is to generate a professional Markdown changelog from a list of git commit messages.
The output should be categorized (e.g., Features, Bug Fixes, Chores, Documentation).
Use Conventional Commits patterns (feat, fix, chore, docs, refactor) to help categorization.
If some commits are vague, group them under "Other Improvements".
Final output should be ONLY the Markdown content. Do not include thought blocks or metadata.`

	userPrompt := fmt.Sprintf("Generation version: %s\n\nRecent Commits:\n%s", version, commitList)

	response, _, err := ai.GenerateResponse(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "", err
	}

	// Clean up any remaining thought blocks (extra safety)
	response = strings.TrimSpace(response)
	return response, nil
}
