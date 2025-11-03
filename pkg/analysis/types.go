package analysis

import (
	"fmt"
	"strings"
	"time"
)

// BundleAnalysis represents complete analysis results for a bundle
// image.
type BundleAnalysis struct {
	BundleRef  ImageRef      `json:"bundle_ref"`
	BundleInfo *ImageInfo    `json:"bundle_info,omitempty"`
	Stream     string        `json:"stream"` // Stream detected from bundle (ystream/zstream)
	Images     []ImageResult `json:"images"`
	Summary    Summary       `json:"summary"`
}

// ImageResult contains analysis results for a single image.
type ImageResult struct {
	Reference  string       `json:"reference"`
	TenantRef  string       `json:"tenant_ref,omitempty"` // Tenant workspace reference if found there
	Accessible bool         `json:"accessible"`
	Registry   RegistryType `json:"registry"`
	Info       *ImageInfo   `json:"info,omitempty"`
	Error      string       `json:"error,omitempty"`
}

// ImageInfo holds extracted metadata from image labels and manifest.
type ImageInfo struct {
	Created      *time.Time `json:"created,omitempty"`
	Version      string     `json:"version,omitempty"`
	CSVVersion   string     `json:"csv_version,omitempty"`
	CSVCreatedAt string     `json:"csv_created_at,omitempty"`
	GitCommit    string     `json:"git_commit,omitempty"`
	GitURL       string     `json:"git_url,omitempty"`
	CommitDate   *time.Time `json:"commit_date,omitempty"`
	PRNumber     int        `json:"pr_number,omitempty"`
	PRTitle      string     `json:"pr_title,omitempty"`
}

// Summary provides aggregate statistics from the analysis.
type Summary struct {
	TotalImages        int `json:"total_images"`
	AccessibleImages   int `json:"accessible_images"`
	DownstreamImages   int `json:"downstream_images"`
	TenantImages       int `json:"tenant_images"`
	InaccessibleImages int `json:"inaccessible_images"`
}

// RegistryType indicates where an image was found.
type RegistryType string

const (
	DownstreamRegistry RegistryType = "downstream"
	TenantWorkspace    RegistryType = "tenant"
	NotAccessible      RegistryType = "inaccessible"
)

// ImageRef represents a parsed container image reference.
type ImageRef struct {
	Registry string
	Repo     string
	Tag      string
	Digest   string
}

// String returns the full image reference string.
func (r ImageRef) String() string {
	if r.Digest != "" {
		return fmt.Sprintf("%s/%s@%s", r.Registry, r.Repo, r.Digest)
	}
	if r.Tag != "" {
		return fmt.Sprintf("%s/%s:%s", r.Registry, r.Repo, r.Tag)
	}
	return fmt.Sprintf("%s/%s", r.Registry, r.Repo)
}

// ParseImageRef parses a container image reference string into
// components.
func ParseImageRef(ref string) (ImageRef, error) {
	if ref == "" {
		return ImageRef{}, fmt.Errorf("empty image reference")
	}

	// Handle digest-based references (preferred).
	if strings.Contains(ref, "@sha256:") {
		parts := strings.Split(ref, "@")
		if len(parts) != 2 {
			return ImageRef{}, fmt.Errorf("invalid digest reference format: %s", ref)
		}

		registryRepo := parts[0]
		digest := parts[1]

		slashIndex := strings.Index(registryRepo, "/")
		if slashIndex == -1 {
			return ImageRef{}, fmt.Errorf("invalid reference format, missing registry: %s", ref)
		}

		result := ImageRef{
			Registry: registryRepo[:slashIndex],
			Repo:     registryRepo[slashIndex+1:],
			Digest:   digest,
		}

		if result.Digest == "" {
			return ImageRef{}, fmt.Errorf("digest-based reference has empty digest: %s", ref)
		}

		return result, nil
	}

	var registry, repo, tag string
	slashIndex := strings.Index(ref, "/")
	if slashIndex == -1 {
		return ImageRef{}, fmt.Errorf("invalid reference format, missing registry: %s", ref)
	}

	registry = ref[:slashIndex]
	repoWithTag := ref[slashIndex+1:]

	if colonIndex := strings.LastIndex(repoWithTag, ":"); colonIndex != -1 {
		repo = repoWithTag[:colonIndex]
		tag = repoWithTag[colonIndex+1:]
	} else {
		repo = repoWithTag
	}

	result := ImageRef{
		Registry: registry,
		Repo:     repo,
		Tag:      tag,
	}

	if result.Tag == "" {
		return ImageRef{}, fmt.Errorf("image reference missing tag (and not digest-based): %s", ref)
	}

	return result, nil
}

// DetectStreamFromRepo detects the stream (ystream/zstream) from a repository name.
// Returns "ystream" as default if no stream indicator is found.
func DetectStreamFromRepo(repo string) string {
	if strings.Contains(repo, "-zstream") {
		return "zstream"
	}
	if strings.Contains(repo, "-ystream") {
		return "ystream"
	}
	// Default to ystream for backwards compatibility
	return "ystream"
}

// ConvertToTenantWorkspace converts a downstream registry reference
// to tenant workspace using the specified stream.
func (r ImageRef) ConvertToTenantWorkspace(stream string) (ImageRef, error) {
	if r.Registry != "registry.redhat.io" {
		return ImageRef{}, fmt.Errorf("can only convert downstream registry references")
	}

	// Convert registry.redhat.io/bpfman/component-name to
	// quay.io/redhat-user-workloads/ocp-bpfman-tenant/component-name-{stream}.
	if !strings.HasPrefix(r.Repo, "bpfman/") {
		return ImageRef{}, fmt.Errorf("unsupported repository path for tenant conversion: %s", r.Repo)
	}

	component := strings.TrimPrefix(r.Repo, "bpfman/")

	// Map downstream component names to tenant workspace names
	var tenantComponent string
	switch component {
	case "bpfman":
		tenantComponent = fmt.Sprintf("bpfman-daemon-%s", stream)
	case "bpfman-rhel9-operator":
		tenantComponent = fmt.Sprintf("bpfman-operator-%s", stream)
	case "bpfman-agent":
		tenantComponent = fmt.Sprintf("bpfman-agent-%s", stream)
	case "bpfman-operator-bundle":
		tenantComponent = fmt.Sprintf("bpfman-operator-bundle-%s", stream)
	default:
		// Fallback: append -{stream}
		tenantComponent = fmt.Sprintf("%s-%s", component, stream)
	}

	tenantRepo := fmt.Sprintf("redhat-user-workloads/ocp-bpfman-tenant/%s", tenantComponent)

	return ImageRef{
		Registry: "quay.io",
		Repo:     tenantRepo,
		Tag:      r.Tag,
		Digest:   r.Digest,
	}, nil
}
