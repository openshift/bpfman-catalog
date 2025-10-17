package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/containers/image/v5/types"
)

// ExtractImageMetadata performs detailed metadata extraction from
// image labels.
func ExtractImageMetadata(ctx context.Context, imageRef ImageRef) (*ImageInfo, error) {
	info, err := inspectImageRef(ctx, imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image: %w", err)
	}

	return extractMetadataFromLabels(info), nil
}

// extractMetadataFromLabels extracts metadata from image labels.
func extractMetadataFromLabels(info *types.ImageInspectInfo) *ImageInfo {
	if info == nil {
		return &ImageInfo{}
	}

	if info.Labels == nil {
		return &ImageInfo{
			Created: info.Created,
		}
	}

	labels := info.Labels
	metadata := &ImageInfo{
		Created: info.Created,
	}

	metadata.Version = extractVersion(labels)
	metadata.GitCommit = extractGitCommit(labels)
	metadata.GitURL = extractGitURL(labels)
	metadata.PRNumber, metadata.PRTitle = extractPRInfo(labels)

	if metadata.GitCommit != "" && metadata.GitURL != "" {
		if commitDate := fetchCommitDate(metadata.GitURL, metadata.GitCommit); commitDate != nil {
			metadata.CommitDate = commitDate
		}
	}

	return metadata
}

// extractVersion attempts to find version information from various label patterns.
func extractVersion(labels map[string]string) string {
	versionKeys := []string{
		"version",
		"io.openshift.tags",
		"io.k8s.display-name",
		"summary",
		"name",
	}

	for _, key := range versionKeys {
		if value := labels[key]; value != "" {
			if version := extractVersionFromString(value); version != "" {
				return version
			}
		}
	}

	return ""
}

// extractVersionFromString attempts to extract version patterns from
// a string.
func extractVersionFromString(s string) string {
	patterns := []string{
		`v?(\d+\.\d+\.\d+)`,    // v1.2.3 or 1.2.3
		`v?(\d+\.\d+)`,         // v1.2 or 1.2
		`(\d+\.\d+\.\d+-\w+)`,  // 1.2.3-alpha
		`(\d+\.\d+\.\d+\.\d+)`, // 1.2.3.4
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(s); len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}

// extractGitCommit attempts to find git commit information from
// labels.
func extractGitCommit(labels map[string]string) string {
	commitKeys := []string{
		"io.openshift.build.commit.id",
		"vcs-ref",
		"io.openshift.build.commit",
		"git.commit",
		"commit",
	}

	for _, key := range commitKeys {
		if value := labels[key]; value != "" {
			if isValidCommitHash(value) {
				return value
			}
		}
	}

	return ""
}

// extractGitURL attempts to find git repository URL from labels.
func extractGitURL(labels map[string]string) string {
	urlKeys := []string{
		"io.openshift.build.source-location",
		"vcs-url",
		"io.openshift.build.source",
		"git.url",
		"source",
	}

	for _, key := range urlKeys {
		if value := labels[key]; value != "" {
			url := cleanGitURL(value)
			if url != "" {
				return url
			}
		}
	}

	return ""
}

// extractPRInfo attempts to extract PR number and title from various
// sources.
func extractPRInfo(labels map[string]string) (int, string) {
	prKeys := []string{
		"io.openshift.build.name",
		"build.name",
		"name",
	}

	for _, key := range prKeys {
		if value := labels[key]; value != "" {
			if prNum := extractPRNumber(value); prNum > 0 {
				return prNum, value
			}
		}
	}

	return 0, ""
}

// isValidCommitHash checks if a string looks like a git commit hash.
func isValidCommitHash(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}

	matched, _ := regexp.MatchString(`^[a-fA-F0-9]+$`, s)
	return matched
}

// cleanGitURL cleans and normalises a git URL.
func cleanGitURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	url := strings.TrimSuffix(rawURL, ".git")

	if strings.Contains(url, "github.com") {
		return url
	}

	return rawURL
}

// extractPRNumber attempts to extract a PR number from a string.
func extractPRNumber(s string) int {
	patterns := []string{
		`pr-(\d+)`,
		`pull-(\d+)`,
		`#(\d+)`,
		`-(\d+)$`, // Number at the end
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(s); len(matches) > 1 {
			if num, err := strconv.Atoi(matches[1]); err == nil {
				return num
			}
		}
	}

	return 0
}

// fetchCommitDate fetches the commit date from GitHub using the gh CLI.
// Returns nil if gh is not available or the fetch fails.
func fetchCommitDate(gitURL, commitHash string) *time.Time {
	ownerRepo := extractGitHubOwnerRepo(gitURL)
	if ownerRepo == "" {
		return nil
	}

	apiPath := fmt.Sprintf("repos/%s/commits/%s", ownerRepo, commitHash)
	cmd := exec.Command("gh", "api", apiPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}

	var result struct {
		Commit struct {
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil
	}

	if result.Commit.Committer.Date != "" {
		if t, err := time.Parse(time.RFC3339, result.Commit.Committer.Date); err == nil {
			return &t
		}
	}

	return nil
}

// extractGitHubOwnerRepo extracts "owner/repo" from a GitHub URL.
func extractGitHubOwnerRepo(gitURL string) string {
	if !strings.Contains(gitURL, "github.com") {
		return ""
	}

	// Handle https://github.com/owner/repo or git@github.com:owner/repo.git
	gitURL = strings.TrimSuffix(gitURL, ".git")

	re := regexp.MustCompile(`github\.com[:/]([^/]+/[^/]+)`)
	matches := re.FindStringSubmatch(gitURL)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}
