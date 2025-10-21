package analysis

import (
	"context"
	"fmt"
	"strings"

	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image/execregistry"
	"github.com/sirupsen/logrus"
)

// AnalyseBundle performs analysis of a bundle image.
func AnalyseBundle(ctx context.Context, bundleRefStr string) (*BundleAnalysis, error) {
	resolvedRefStr := bundleRefStr
	if !strings.Contains(bundleRefStr, "@sha256:") {
		logrus.Infof("Resolving tag reference to digest: %s", bundleRefStr)
		resolved, err := ResolveToDigest(ctx, bundleRefStr)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve tag to digest: %w", err)
		}
		resolvedRefStr = resolved
		logrus.Infof("Resolved to digest: %s", resolvedRefStr)
	}

	bundleRef, err := ParseImageRef(resolvedRefStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bundle reference: %w", err)
	}

	logrus.Debugf("Parsed bundle ref: %s", bundleRef.String())

	analysis := &BundleAnalysis{
		BundleRef: bundleRef,
		Images:    []ImageResult{},
	}

	logrus.Infof("Inspecting bundle metadata from %s", bundleRef.String())
	bundleInfo, err := extractBundleMetadata(ctx, bundleRef)
	if err != nil {
		return nil, fmt.Errorf("failed to extract bundle metadata: %w", err)
	}
	analysis.BundleInfo = bundleInfo

	logrus.Infof("Extracting image references from bundle")
	imageRefs, err := ExtractImageReferences(ctx, bundleRef)
	if err != nil {
		return nil, fmt.Errorf("failed to extract image references: %w", err)
	}

	logrus.Infof("Found %d image references, inspecting each", len(imageRefs))
	imageResults := make([]ImageResult, len(imageRefs))
	for i, ref := range imageRefs {
		logrus.Infof("Inspecting image %d/%d: %s", i+1, len(imageRefs), ref)
		result, err := InspectImage(ctx, ref)
		if err != nil {
			imageResults[i] = ImageResult{
				Reference:  ref,
				Accessible: false,
				Registry:   NotAccessible,
				Error:      fmt.Sprintf("inspection failed: %v", err),
			}
		} else {
			imageResults[i] = *result
		}
	}
	analysis.Images = imageResults
	analysis.Summary = CalculateSummary(imageResults)

	return analysis, nil
}

// extractBundleMetadata extracts metadata from the bundle image
// itself.
func extractBundleMetadata(ctx context.Context, bundleRef ImageRef) (*ImageInfo, error) {
	var activeRef ImageRef = bundleRef
	info, err := ExtractImageMetadata(ctx, bundleRef)
	if err != nil {
		tenantRef, err := bundleRef.ConvertToTenantWorkspace()
		if err != nil {
			return nil, fmt.Errorf("bundle not accessible and cannot convert to tenant workspace: %w", err)
		}

		info, err = ExtractImageMetadata(ctx, tenantRef)
		if err != nil {
			return nil, fmt.Errorf("bundle not accessible in any registry: %w", err)
		}
		activeRef = tenantRef
	}

	// Extract CSV metadata from bundle
	csvMetadata, err := extractCSVMetadataFromBundle(ctx, activeRef)
	if err != nil {
		logrus.WithError(err).Debugf("failed to extract CSV metadata from bundle")
	} else if csvMetadata != nil {
		if csvMetadata.Version != "" {
			info.CSVVersion = csvMetadata.Version
		}
		if csvMetadata.CreatedAt != "" {
			info.CSVCreatedAt = csvMetadata.CreatedAt
		}
	}

	return info, nil
}

// extractCSVMetadataFromBundle extracts the CSV metadata using a temporary registry.
func extractCSVMetadataFromBundle(ctx context.Context, bundleRef ImageRef) (*CSVMetadata, error) {
	logrus.Debugf("Extracting CSV metadata from bundle: %s", bundleRef.String())

	logrus.SetLevel(logrus.WarnLevel)
	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.WarnLevel)

	registry, err := execregistry.NewRegistry(containertools.PodmanTool, logger)
	if err != nil {
		return nil, fmt.Errorf("creating image registry: %w", err)
	}
	defer registry.Destroy()

	return ExtractCSVMetadata(ctx, bundleRef, registry)
}

// AnalyseConfig holds configuration options for bundle analysis.
type AnalyseConfig struct {
	ShowAll bool // Include inaccessible images in results.
}
