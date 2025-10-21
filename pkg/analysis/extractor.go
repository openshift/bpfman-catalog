package analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/action/migrations"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image"
	"github.com/operator-framework/operator-registry/pkg/image/execregistry"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
)

// ExtractImageReferences extracts all image references from a bundle
// image by directly inspecting it.
func ExtractImageReferences(ctx context.Context, bundleRef ImageRef) ([]string, error) {
	logrus.Debugf("ExtractImageReferences from bundle: %s", bundleRef.String())

	logrus.SetLevel(logrus.WarnLevel)

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.WarnLevel)

	registry, err := execregistry.NewRegistry(containertools.PodmanTool, logger)
	if err != nil {
		return nil, fmt.Errorf("creating image registry: %w", err)
	}
	defer registry.Destroy()

	migs, err := migrations.NewMigrations("bundle-object-to-csv-metadata")
	if err != nil {
		return nil, fmt.Errorf("creating migrations: %w", err)
	}

	r := action.Render{
		Refs:           []string{bundleRef.String()},
		Registry:       registry,
		AllowedRefMask: action.RefBundleImage,
		Migrations:     migs,
	}

	logrus.Debugf("Rendering bundle to extract image references")
	cfg, err := r.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("rendering bundle: %w", err)
	}

	var images []string
	for _, bundle := range cfg.Bundles {
		if bundle.Image != "" {
			logrus.Debugf("Found bundle image: %s", bundle.Image)
			images = append(images, bundle.Image)
		}

		for _, relatedImage := range bundle.RelatedImages {
			if relatedImage.Image != "" {
				logrus.Debugf("Found relatedImage: %s", relatedImage.Image)
				images = append(images, relatedImage.Image)
			}
		}
	}

	configmapImages, err := extractConfigMapImages(ctx, bundleRef, registry)
	if err != nil {
		logrus.WithError(err).Warn("failed to extract configmap images")
	} else {
		images = append(images, configmapImages...)
	}

	return deduplicateStrings(images), nil
}

// deduplicateStrings removes duplicate strings from a slice.
func deduplicateStrings(slice []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// extractConfigMapImages extracts image references from the bpfman-config ConfigMap
// in the bundle manifests. These images (daemon and agent) are not tracked in
// relatedImages but are configured via ConfigMap at runtime.
func extractConfigMapImages(ctx context.Context, bundleRef ImageRef, registry image.Registry) ([]string, error) {
	tmpDir, err := os.MkdirTemp("", "bundle-manifests-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	ref := image.SimpleReference(bundleRef.String())
	if err := registry.Unpack(ctx, ref, tmpDir); err != nil {
		return nil, fmt.Errorf("unpacking bundle image: %w", err)
	}

	configmapPath := filepath.Join(tmpDir, "manifests", "bpfman-config_v1_configmap.yaml")
	data, err := os.ReadFile(configmapPath)
	if err != nil {
		return nil, fmt.Errorf("reading configmap file: %w", err)
	}

	var images []string

	daemonPattern := regexp.MustCompile(`bpfman\.image:\s*["']?([^\s"']+)["']?`)
	if match := daemonPattern.FindSubmatch(data); len(match) > 1 {
		images = append(images, string(match[1]))
	}

	agentPattern := regexp.MustCompile(`bpfman\.agent\.image:\s*["']?([^\s"']+)["']?`)
	if match := agentPattern.FindSubmatch(data); len(match) > 1 {
		images = append(images, string(match[1]))
	}

	return images, nil
}

// CSVMetadata holds extracted metadata from the ClusterServiceVersion.
type CSVMetadata struct {
	Version   string
	CreatedAt string
}

// ExtractCSVMetadata extracts version and createdAt from the ClusterServiceVersion in a bundle image.
func ExtractCSVMetadata(ctx context.Context, bundleRef ImageRef, registry image.Registry) (*CSVMetadata, error) {
	tmpDir, err := os.MkdirTemp("", "bundle-csv-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	ref := image.SimpleReference(bundleRef.String())
	if err := registry.Unpack(ctx, ref, tmpDir); err != nil {
		return nil, fmt.Errorf("unpacking bundle image: %w", err)
	}

	manifestDir := filepath.Join(tmpDir, "manifests")
	entries, err := os.ReadDir(manifestDir)
	if err != nil {
		return nil, fmt.Errorf("reading manifests directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.Contains(strings.ToLower(entry.Name()), "clusterserviceversion") {
			csvPath := filepath.Join(manifestDir, entry.Name())
			data, err := os.ReadFile(csvPath)
			if err != nil {
				logrus.WithError(err).Debugf("failed to read CSV file: %s", entry.Name())
				continue
			}

			var csv struct {
				Spec struct {
					Version string `yaml:"version"`
				} `yaml:"spec"`
				Metadata struct {
					Annotations map[string]string `yaml:"annotations"`
				} `yaml:"metadata"`
			}

			if err := yaml.Unmarshal(data, &csv); err != nil {
				logrus.WithError(err).Debugf("failed to parse CSV YAML: %s", entry.Name())
				continue
			}

			metadata := &CSVMetadata{
				Version:   csv.Spec.Version,
				CreatedAt: csv.Metadata.Annotations["createdAt"],
			}

			if metadata.Version != "" || metadata.CreatedAt != "" {
				logrus.Debugf("Found CSV metadata: version=%s, createdAt=%s", metadata.Version, metadata.CreatedAt)
				return metadata, nil
			}
		}
	}

	return nil, nil
}

// ResolveToDigest resolves an image reference to a digest-based reference.
// If the reference already uses a digest (@sha256:...), it returns it unchanged.
// If the reference uses a tag (:latest, :v1.0, etc.), it inspects the image
// and returns a digest-based reference for reproducibility.
func ResolveToDigest(ctx context.Context, imageRef string) (string, error) {
	if strings.Contains(imageRef, "@sha256:") {
		return imageRef, nil
	}

	named, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		return "", fmt.Errorf("parsing image reference: %w", err)
	}

	ref, err := docker.ParseReference("//" + imageRef)
	if err != nil {
		return "", fmt.Errorf("parsing docker reference: %w", err)
	}

	sys := &types.SystemContext{}
	manifestDigest, err := docker.GetDigest(ctx, sys, ref)
	if err != nil {
		return "", fmt.Errorf("getting image digest: %w", err)
	}

	return fmt.Sprintf("%s@%s", reference.FamiliarName(named), manifestDigest.String()), nil
}
