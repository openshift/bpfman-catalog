package analysis

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FormatResult formats analysis results according to the specified
// format.
func FormatResult(analysis *BundleAnalysis, format string) (string, error) {
	switch strings.ToLower(format) {
	case "json":
		return formatJSON(analysis)
	case "text", "":
		return formatText(analysis), nil
	default:
		return "", fmt.Errorf("unsupported format: %s (supported: text, json)", format)
	}
}

// formatJSON returns JSON-formatted analysis results.
func formatJSON(analysis *BundleAnalysis) (string, error) {
	data, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(data), nil
}

// formatText returns human-readable text-formatted analysis results.
func formatText(analysis *BundleAnalysis) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Bundle: %s\n", analysis.BundleRef.String()))

	if analysis.BundleInfo != nil {
		if analysis.BundleInfo.Created != nil {
			b.WriteString(fmt.Sprintf("  Created: %s\n", analysis.BundleInfo.Created.Format(time.RFC3339)))
		}
		if analysis.BundleInfo.Version != "" {
			b.WriteString(fmt.Sprintf("  Image version (label): %s\n", analysis.BundleInfo.Version))
		}
		if analysis.BundleInfo.CSVVersion != "" {
			b.WriteString(fmt.Sprintf("  ClusterServiceVersion: %s\n", analysis.BundleInfo.CSVVersion))
		}
		if analysis.BundleInfo.CSVCreatedAt != "" {
			b.WriteString(fmt.Sprintf("  CSV Created: %s\n", analysis.BundleInfo.CSVCreatedAt))
		}
		if analysis.BundleInfo.GitCommit != "" && analysis.BundleInfo.GitURL != "" {
			commitURL := buildCommitURL(analysis.BundleInfo.GitURL, analysis.BundleInfo.GitCommit)
			b.WriteString(fmt.Sprintf("  Git: %s\n", commitURL))
		}
		if analysis.BundleInfo.PRNumber > 0 {
			prURL := buildPRURL(analysis.BundleInfo.GitURL, analysis.BundleInfo.PRNumber)
			if prURL != "" {
				title := analysis.BundleInfo.PRTitle
				if title == "" {
					title = fmt.Sprintf("PR #%d", analysis.BundleInfo.PRNumber)
				}
				b.WriteString(fmt.Sprintf("  PR: %s - %s\n", prURL, title))
			}
		}
	}
	b.WriteString("\n")

	imageCount := len(analysis.Images)
	if imageCount == 0 {
		b.WriteString("No images found in bundle.\n")
	} else {
		b.WriteString(fmt.Sprintf("Images (%d found):\n\n", imageCount))
		for _, img := range analysis.Images {
			componentLabel := identifyComponent(img.Reference)
			if componentLabel != "" {
				b.WriteString(fmt.Sprintf("=== %s ===\n", componentLabel))
			}
			b.WriteString(formatImageResult(img))
		}
	}

	b.WriteString(formatSummary(analysis.Summary))

	return b.String()
}

// formatImageResult formats a single image result.
func formatImageResult(img ImageResult) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("  %s\n", img.Reference))

	if !img.Accessible {
		if img.Error != "" {
			b.WriteString(fmt.Sprintf("    ✗ %s\n", img.Error))
		} else {
			b.WriteString("    ✗ Not accessible\n")
		}
		b.WriteString("\n")
		return b.String()
	}

	// Registry status.
	switch img.Registry {
	case DownstreamRegistry:
		b.WriteString("    ✓ Published in downstream registry (registry.redhat.io)\n")
	case TenantWorkspace:
		b.WriteString("    ⚠ Only in tenant workspace (not yet published downstream)\n")
		if img.TenantRef != "" {
			b.WriteString(fmt.Sprintf("    Source: %s\n", img.TenantRef))
		}
	default:
		b.WriteString("    ✗ Registry status unknown\n")
	}

	// Metadata.
	if img.Info != nil {
		if img.Info.Created != nil {
			b.WriteString(fmt.Sprintf("    Created: %s\n", img.Info.Created.Format(time.RFC3339)))
		}
		if img.Info.Version != "" {
			b.WriteString(fmt.Sprintf("    Image version (label): %s\n", img.Info.Version))
		}
		if img.Info.GitCommit != "" && img.Info.GitURL != "" {
			commitURL := buildCommitURL(img.Info.GitURL, img.Info.GitCommit)
			b.WriteString(fmt.Sprintf("    Git: %s\n", commitURL))
			if img.Info.CommitDate != nil {
				b.WriteString(fmt.Sprintf("    Commit Date: %s\n", img.Info.CommitDate.Format(time.RFC3339)))
			}
		}
		if img.Info.PRNumber > 0 {
			prURL := buildPRURL(img.Info.GitURL, img.Info.PRNumber)
			if prURL != "" {
				b.WriteString(fmt.Sprintf("    PR: %s\n", prURL))
			}
		}
	}

	b.WriteString("\n")
	return b.String()
}

// formatSummary formats the analysis summary.
func formatSummary(summary Summary) string {
	if summary.TotalImages == 0 {
		return "Summary: No images analysed.\n"
	}

	parts := []string{
		fmt.Sprintf("%d images", summary.TotalImages),
	}

	if summary.AccessibleImages > 0 {
		parts = append(parts, fmt.Sprintf("%d accessible", summary.AccessibleImages))
	}

	if summary.DownstreamImages > 0 {
		parts = append(parts, fmt.Sprintf("%d downstream", summary.DownstreamImages))
	}

	if summary.TenantImages > 0 {
		parts = append(parts, fmt.Sprintf("%d tenant workspace", summary.TenantImages))
	}

	if summary.InaccessibleImages > 0 {
		parts = append(parts, fmt.Sprintf("%d inaccessible", summary.InaccessibleImages))
	}

	return fmt.Sprintf("Summary: %s\n", strings.Join(parts, ", "))
}

// buildCommitURL constructs a commit URL from git URL and commit
// hash.
func buildCommitURL(gitURL, commit string) string {
	if gitURL == "" || commit == "" {
		return fmt.Sprintf("%s (commit: %s)", gitURL, commit)
	}

	// Handle GitHub URLs.
	if strings.Contains(gitURL, "github.com") {
		baseURL := strings.TrimSuffix(gitURL, ".git")
		return fmt.Sprintf("%s/commit/%s", baseURL, commit)
	}

	return fmt.Sprintf("%s (commit: %s)", gitURL, commit)
}

// buildPRURL constructs a PR URL from git URL and PR number.
func buildPRURL(gitURL string, prNumber int) string {
	if gitURL == "" || prNumber <= 0 {
		return ""
	}

	if strings.Contains(gitURL, "github.com") {
		baseURL := strings.TrimSuffix(gitURL, ".git")
		return fmt.Sprintf("%s/pull/%d", baseURL, prNumber)
	}

	return ""
}

// identifyComponent identifies the component type based on image reference.
func identifyComponent(imageRef string) string {
	lowerRef := strings.ToLower(imageRef)

	if strings.Contains(lowerRef, "bundle") {
		return "Bundle Image"
	}
	if strings.Contains(lowerRef, "agent") {
		return "Bpfman Agent Image"
	}
	if strings.Contains(lowerRef, "operator") {
		return "Operator Image"
	}
	// Match bpfman without any suffix (daemon/rust component)
	if strings.Contains(lowerRef, "/bpfman@") || strings.Contains(lowerRef, "/bpfman:") {
		return "Bpfman Daemon (Rust) Image"
	}

	return ""
}
