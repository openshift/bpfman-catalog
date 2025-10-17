package analysis

import (
	"context"
	"fmt"
	"strings"

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

	logrus.Debugf("Extracting bundle metadata")
	bundleInfo, err := extractBundleMetadata(ctx, bundleRef)
	if err != nil {
		return nil, fmt.Errorf("failed to extract bundle metadata: %w", err)
	}
	analysis.BundleInfo = bundleInfo

	logrus.Debugf("Extracting image references from bundle")
	imageRefs, err := ExtractImageReferences(ctx, bundleRef)
	if err != nil {
		return nil, fmt.Errorf("failed to extract image references: %w", err)
	}

	logrus.Debugf("Found %d image references", len(imageRefs))
	imageResults := make([]ImageResult, len(imageRefs))
	for i, ref := range imageRefs {
		logrus.Debugf("Inspecting image %d/%d: %s", i+1, len(imageRefs), ref)
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
	if info, err := ExtractImageMetadata(ctx, bundleRef); err == nil {
		return info, nil
	}

	tenantRef, err := bundleRef.ConvertToTenantWorkspace()
	if err != nil {
		return nil, fmt.Errorf("bundle not accessible and cannot convert to tenant workspace: %w", err)
	}

	info, err := ExtractImageMetadata(ctx, tenantRef)
	if err != nil {
		return nil, fmt.Errorf("bundle not accessible in any registry: %w", err)
	}

	return info, nil
}

// AnalyseConfig holds configuration options for bundle analysis.
type AnalyseConfig struct {
	ShowAll bool // Include inaccessible images in results.
}
