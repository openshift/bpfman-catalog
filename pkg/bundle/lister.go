package bundle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
)

const (
	defaultRegistry    = "quay.io/redhat-user-workloads"
	defaultTenant      = "ocp-bpfman-tenant"
	defaultBundleRepo  = "bpfman-operator-bundle-ystream"
	maxConcurrency     = 10
	gitCommitTagLength = 40
)

// BundleMetadata contains metadata about a bundle image.
type BundleMetadata struct {
	Image     string        `json:"image"`
	Tag       string        `json:"tag"`
	Digest    digest.Digest `json:"digest"`
	BuildDate string        `json:"build_date"`
	Version   string        `json:"version"`
	Created   time.Time     `json:"created"`
}

// BundleRef represents a bundle image reference.
type BundleRef struct {
	Registry string
	Tenant   string
	Repo     string
}

// String returns the full image reference.
func (r BundleRef) String() string {
	return fmt.Sprintf("%s/%s/%s", r.Registry, r.Tenant, r.Repo)
}

// NewDefaultBundleRef creates a bundle reference with default values.
func NewDefaultBundleRef() BundleRef {
	return BundleRef{
		Registry: defaultRegistry,
		Tenant:   defaultTenant,
		Repo:     defaultBundleRepo,
	}
}

// isGitCommitTag checks if a tag is a 40-character git commit SHA.
func isGitCommitTag(tag string) bool {
	if len(tag) != gitCommitTagLength {
		return false
	}
	for _, c := range tag {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// filterGitCommitTags filters tags to only include git commit SHAs.
func filterGitCommitTags(tags []string) []string {
	var commitTags []string
	for _, tag := range tags {
		if isGitCommitTag(tag) {
			commitTags = append(commitTags, tag)
		}
	}
	return commitTags
}

// fetchTags fetches all tags for a bundle repository.
func fetchTags(ctx context.Context, bundleRef BundleRef) ([]string, error) {
	ref, err := docker.ParseReference(fmt.Sprintf("//%s", bundleRef.String()))
	if err != nil {
		return nil, fmt.Errorf("parsing reference %s: %w", bundleRef, err)
	}

	sys := &types.SystemContext{
		OSChoice:           "linux",
		ArchitectureChoice: "amd64",
	}

	tags, err := docker.GetRepositoryTags(ctx, sys, ref)
	if err != nil {
		return nil, fmt.Errorf("fetching tags for %s: %w", bundleRef, err)
	}

	return tags, nil
}

// fetchBundleMetadata fetches metadata for a specific bundle tag.
func fetchBundleMetadata(ctx context.Context, bundleRef BundleRef, tag string) (*BundleMetadata, error) {
	taggedRef := fmt.Sprintf("%s:%s", bundleRef.String(), tag)
	ref, err := docker.ParseReference(fmt.Sprintf("//%s", taggedRef))
	if err != nil {
		return nil, fmt.Errorf("parsing reference %s: %w", taggedRef, err)
	}

	sys := &types.SystemContext{
		OSChoice:           "linux",
		ArchitectureChoice: "amd64",
	}

	img, err := ref.NewImage(ctx, sys)
	if err != nil {
		return nil, fmt.Errorf("creating image for %s: %w", taggedRef, err)
	}
	defer img.Close()

	manifestBlob, _, err := img.Manifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest for %s: %w", taggedRef, err)
	}

	manifestDigest, err := manifest.Digest(manifestBlob)
	if err != nil {
		return nil, fmt.Errorf("computing digest for %s: %w", taggedRef, err)
	}

	inspect, err := img.Inspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("inspecting image %s: %w", taggedRef, err)
	}

	metadata := &BundleMetadata{
		Image:  taggedRef,
		Tag:    tag,
		Digest: manifestDigest,
	}

	if inspect.Labels != nil {
		metadata.BuildDate = inspect.Labels["build-date"]
		metadata.Version = inspect.Labels["version"]
	}

	if inspect.Created != nil {
		metadata.Created = *inspect.Created
	}

	if metadata.BuildDate == "" {
		return nil, fmt.Errorf("no build date found for %s", taggedRef)
	}

	return metadata, nil
}

// fetchAllBundleMetadata fetches metadata for all tags concurrently.
func fetchAllBundleMetadata(ctx context.Context, bundleRef BundleRef, tags []string) ([]*BundleMetadata, error) {
	var wg sync.WaitGroup
	results := make(chan struct {
		metadata *BundleMetadata
		err      error
	}, len(tags))
	semaphore := make(chan struct{}, maxConcurrency)

	for _, tag := range tags {
		wg.Add(1)
		go func(tag string) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				results <- struct {
					metadata *BundleMetadata
					err      error
				}{nil, fmt.Errorf("tag %s: %w", tag, ctx.Err())}
				return
			default:
			}

			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results <- struct {
					metadata *BundleMetadata
					err      error
				}{nil, fmt.Errorf("tag %s: %w", tag, ctx.Err())}
				return
			}

			metadata, err := fetchBundleMetadata(ctx, bundleRef, tag)
			results <- struct {
				metadata *BundleMetadata
				err      error
			}{metadata, err}
		}(tag)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var bundles []*BundleMetadata
	var errs []error

	for result := range results {
		if result.err != nil {
			if !errors.Is(result.err, context.Canceled) {
				errs = append(errs, result.err)
			}
		} else if result.metadata != nil {
			bundles = append(bundles, result.metadata)
		}
	}

	if ctx.Err() != nil {
		return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
	}

	if len(bundles) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("failed to fetch any bundles: %v", errors.Join(errs...))
	}

	return bundles, nil
}

// sortByBuildDate sorts bundles by build date (newest first).
func sortByBuildDate(bundles []*BundleMetadata) {
	sort.Slice(bundles, func(i, j int) bool {
		return bundles[i].BuildDate > bundles[j].BuildDate
	})
}

// ListLatestBundles lists the latest N bundle builds from a
// repository.
func ListLatestBundles(ctx context.Context, bundleRef BundleRef, limit int) ([]*BundleMetadata, error) {
	tags, err := fetchTags(ctx, bundleRef)
	if err != nil {
		return nil, fmt.Errorf("fetching tags: %w", err)
	}

	commitTags := filterGitCommitTags(tags)
	if len(commitTags) == 0 {
		return nil, fmt.Errorf("no git commit tags found among %d tags", len(tags))
	}

	bundles, err := fetchAllBundleMetadata(ctx, bundleRef, commitTags)
	if err != nil {
		return nil, fmt.Errorf("fetching metadata: %w", err)
	}

	if len(bundles) == 0 {
		return nil, errors.New("no bundles with metadata found")
	}

	sortByBuildDate(bundles)

	if limit > len(bundles) {
		limit = len(bundles)
	}

	return bundles[:limit], nil
}

// ParseBundleRef parses a bundle image reference string into
// components.
func ParseBundleRef(imageRef string) (BundleRef, error) {
	imageRef = strings.TrimPrefix(imageRef, "docker://")

	if idx := strings.LastIndex(imageRef, ":"); idx != -1 && !strings.Contains(imageRef[idx:], "/") {
		imageRef = imageRef[:idx]
	}
	if idx := strings.LastIndex(imageRef, "@"); idx != -1 {
		imageRef = imageRef[:idx]
	}

	parts := strings.Split(imageRef, "/")
	if len(parts) < 3 {
		return BundleRef{}, fmt.Errorf("invalid image reference format: %s (expected registry/tenant/repo)", imageRef)
	}

	if len(parts) == 4 && parts[0] == "quay.io" && parts[1] == "redhat-user-workloads" {
		return BundleRef{
			Registry: fmt.Sprintf("%s/%s", parts[0], parts[1]),
			Tenant:   parts[2],
			Repo:     parts[3],
		}, nil
	}

	if len(parts) == 3 {
		return BundleRef{
			Registry: parts[0],
			Tenant:   parts[1],
			Repo:     parts[2],
		}, nil
	}

	return BundleRef{}, fmt.Errorf("unsupported image reference format: %s", imageRef)
}
