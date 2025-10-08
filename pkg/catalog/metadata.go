package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image"
	"github.com/operator-framework/operator-registry/pkg/image/execregistry"
	"github.com/sirupsen/logrus"
)

// ImageMetadata contains metadata extracted from an image reference.
type ImageMetadata struct {
	OriginalRef    string        // Original image reference provided
	Registry       string        // e.g., quay.io
	Namespace      string        // e.g., redhat-user-workloads/ocp-bpfman-tenant
	Repository     string        // e.g., catalog-ystream
	Tag            string        // e.g., latest or v4.19
	Digest         digest.Digest // e.g., sha256:abc123...
	ShortDigest    string        // First 8 chars of digest
	CatalogType    string        // e.g., catalog-ystream, catalog-zstream
	DefaultChannel string        // Default channel from catalog
	Channels       []string      // Available channels
}

// ExtractMetadata extracts metadata from an image reference. If the
// reference uses a tag, it will resolve it to a digest.
func ExtractMetadata(ctx context.Context, imageRef string) (*ImageMetadata, error) {
	meta := &ImageMetadata{
		OriginalRef: imageRef,
	}

	if err := parseImageReference(imageRef, meta); err != nil {
		return nil, fmt.Errorf("parsing image reference: %w", err)
	}

	if meta.Digest == "" {
		if err := fetchDigest(ctx, imageRef, meta); err != nil {
			return nil, fmt.Errorf("fetching digest: %w", err)
		}
	}

	if meta.Digest != "" {
		digestStr := string(meta.Digest)
		if strings.HasPrefix(digestStr, "sha256:") && len(digestStr) >= 15 {
			meta.ShortDigest = digestStr[7:15] // Skip "sha256:" and take 8 chars
		}
	}

	if strings.Contains(meta.Repository, "catalog") {
		parts := strings.Split(meta.Repository, "-")
		if len(parts) >= 2 {
			meta.CatalogType = strings.Join(parts[0:2], "-") // e.g., "catalog-ystream"
		}
	}

	if err := extractChannelInfo(ctx, meta.GetDigestRef(), meta); err != nil {
		return nil, fmt.Errorf("extracting channel information: %w", err)
	}

	return meta, nil
}

// GetDigestRef returns a digest-based image reference.
func (m *ImageMetadata) GetDigestRef() string {
	if m.Digest == "" {
		return m.OriginalRef
	}

	var ref strings.Builder
	ref.WriteString(m.Registry)
	if m.Namespace != "" {
		ref.WriteString("/")
		ref.WriteString(m.Namespace)
	}
	ref.WriteString("/")
	ref.WriteString(m.Repository)
	ref.WriteString("@")
	ref.WriteString(string(m.Digest))

	return ref.String()
}

// parseImageReference parses an image reference into its components.
func parseImageReference(imageRef string, meta *ImageMetadata) error {
	// Handle different formats:
	// quay.io/namespace/repo:tag
	// quay.io/namespace/repo@sha256:digest
	// registry.redhat.io/namespace/repo:tag.

	imageRef = strings.TrimPrefix(imageRef, "docker://")

	var baseRef string
	if idx := strings.Index(imageRef, "@"); idx != -1 {
		baseRef = imageRef[:idx]
		digestStr := imageRef[idx+1:]
		if d, err := digest.Parse(digestStr); err == nil {
			meta.Digest = d
		}
	} else {
		baseRef = imageRef
	}

	parts := strings.Split(baseRef, ":")
	if len(parts) == 2 {
		meta.Tag = parts[1]
		baseRef = parts[0]
	}

	pathParts := strings.Split(baseRef, "/")
	if len(pathParts) < 2 {
		return fmt.Errorf("invalid image reference format: %s", imageRef)
	}

	if strings.Contains(pathParts[0], ".") || strings.Contains(pathParts[0], ":") {
		meta.Registry = pathParts[0]
		pathParts = pathParts[1:]
	} else {
		meta.Registry = "docker.io"
	}

	if len(pathParts) > 0 {
		meta.Repository = pathParts[len(pathParts)-1]
		if len(pathParts) > 1 {
			meta.Namespace = strings.Join(pathParts[:len(pathParts)-1], "/")
		}
	}

	return nil
}

// fetchDigest resolves the digest for an image reference using
// containers/image.
func fetchDigest(ctx context.Context, imageRef string, meta *ImageMetadata) error {
	sysCtx := &types.SystemContext{}

	ref, err := docker.ParseReference("//" + imageRef)
	if err != nil {
		return fmt.Errorf("parsing image reference: %w", err)
	}

	src, err := ref.NewImageSource(ctx, sysCtx)
	if err != nil {
		return fmt.Errorf("creating image source: %w", err)
	}
	defer src.Close()

	manifestBlob, _, err := src.GetManifest(ctx, nil)
	if err != nil {
		return fmt.Errorf("getting manifest: %w", err)
	}

	manifestDigest := digest.FromBytes(manifestBlob)
	meta.Digest = manifestDigest

	return nil
}

// extractChannelInfo inspects the FBC catalog image to determine
// available channels.
func extractChannelInfo(ctx context.Context, imageRef string, meta *ImageMetadata) error {
	tmpDir, err := os.MkdirTemp("", "catalog-extract-*")
	if err != nil {
		return fmt.Errorf("creating temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.ErrorLevel) // Minimise logging noise.

	registry, err := execregistry.NewRegistry(containertools.PodmanTool, logger)
	if err != nil {
		registry, err = execregistry.NewRegistry(containertools.DockerTool, logger)
		if err != nil {
			return fmt.Errorf("creating container registry client: %w", err)
		}
	}
	defer registry.Destroy()

	imgRef := image.SimpleReference(imageRef)

	if err := registry.Pull(ctx, imgRef); err != nil {
		return fmt.Errorf("pulling catalog image: %w", err)
	}

	if err := registry.Unpack(ctx, imgRef, tmpDir); err != nil {
		return fmt.Errorf("unpacking catalog image: %w", err)
	}

	labels, err := registry.Labels(ctx, imgRef)
	if err != nil {
		return fmt.Errorf("getting image labels: %w", err)
	}

	configsDir := "/configs" // Default location.
	if loc, ok := labels[containertools.ConfigsLocationLabel]; ok {
		configsDir = loc
	}

	configsPath := filepath.Join(tmpDir, configsDir)
	if _, err := os.Stat(configsPath); os.IsNotExist(err) {
		return fmt.Errorf("configs directory not found at %s", configsPath)
	}

	cfg, err := declcfg.LoadFS(ctx, os.DirFS(configsPath))
	if err != nil {
		return fmt.Errorf("loading FBC catalog from %s: %w", configsPath, err)
	}

	var bpfmanPackage *declcfg.Package
	for _, pkg := range cfg.Packages {
		if pkg.Name == "bpfman-operator" {
			bpfmanPackage = &pkg
			break
		}
	}

	if bpfmanPackage == nil {
		return fmt.Errorf("bpfman-operator package not found in FBC catalog")
	}

	var channels []declcfg.Channel
	for _, ch := range cfg.Channels {
		if ch.Package == "bpfman-operator" {
			channels = append(channels, ch)
		}
	}

	if len(channels) == 0 {
		return fmt.Errorf("no channels found for bpfman-operator package")
	}

	meta.Channels = make([]string, len(channels))
	for i, channel := range channels {
		meta.Channels[i] = channel.Name
	}

	meta.DefaultChannel = bpfmanPackage.DefaultChannel
	if meta.DefaultChannel == "" && len(meta.Channels) > 0 {
		meta.DefaultChannel = meta.Channels[0]
	}

	return nil
}
