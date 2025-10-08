package analysis

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/types"
)

// InspectImage performs inspection of a single image reference.
func InspectImage(ctx context.Context, imageRefStr string) (*ImageResult, error) {
	imageRef, err := ParseImageRef(imageRefStr)
	if err != nil {
		return &ImageResult{
			Reference:  imageRefStr,
			Accessible: false,
			Registry:   NotAccessible,
			Error:      fmt.Sprintf("invalid image reference: %v", err),
		}, nil
	}

	result := &ImageResult{
		Reference: imageRefStr,
	}

	if info, err := inspectImageRef(ctx, imageRef); err == nil {
		result.Accessible = true
		if imageRef.Registry == "registry.redhat.io" {
			result.Registry = DownstreamRegistry
		} else if imageRef.Registry == "quay.io" && strings.Contains(imageRef.Repo, "redhat-user-workloads") {
			result.Registry = TenantWorkspace
		} else {
			result.Registry = DownstreamRegistry // Default for other registries
		}
		result.Info = convertToImageInfo(info)
		return result, nil
	}

	tenantRef, err := imageRef.ConvertToTenantWorkspace()
	if err != nil {
		result.Accessible = false
		result.Registry = NotAccessible
		result.Error = "not accessible in any registry"
		return result, nil
	}

	if info, err := inspectImageRef(ctx, tenantRef); err == nil {
		result.Accessible = true
		result.Registry = TenantWorkspace
		result.Info = convertToImageInfo(info)
		return result, nil
	}

	result.Accessible = false
	result.Registry = NotAccessible
	result.Error = "not accessible in downstream or tenant registry"
	return result, nil
}

// inspectImageRef inspects a specific image reference and returns
// metadata.
func inspectImageRef(ctx context.Context, imageRef ImageRef) (*types.ImageInspectInfo, error) {
	ref, err := docker.ParseReference("//" + imageRef.String())
	if err != nil {
		return nil, fmt.Errorf("failed to parse reference: %w", err)
	}

	systemCtx := &types.SystemContext{}
	img, err := ref.NewImage(ctx, systemCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create image: %w", err)
	}
	defer img.Close()

	return img.Inspect(ctx)
}

// convertToImageInfo converts types.ImageInspectInfo to our ImageInfo
// structure.
func convertToImageInfo(info *types.ImageInspectInfo) *ImageInfo {
	imageInfo := &ImageInfo{}

	if info.Created != nil {
		imageInfo.Created = info.Created
	}

	if info.Labels != nil {
		imageInfo.Version = info.Labels["version"]

		if commit := info.Labels["io.openshift.build.commit.id"]; commit != "" {
			imageInfo.GitCommit = commit
		} else if commit := info.Labels["vcs-ref"]; commit != "" {
			imageInfo.GitCommit = commit
		}

		if url := info.Labels["io.openshift.build.source-location"]; url != "" {
			imageInfo.GitURL = url
		} else if url := info.Labels["vcs-url"]; url != "" {
			imageInfo.GitURL = url
		}

		if buildName := info.Labels["io.openshift.build.name"]; buildName != "" {
			imageInfo.PRTitle = buildName
		}
	}

	return imageInfo
}

// CalculateSummary generates summary statistics from image results.
func CalculateSummary(results []ImageResult) Summary {
	summary := Summary{
		TotalImages: len(results),
	}

	for _, result := range results {
		if result.Accessible {
			summary.AccessibleImages++
			switch result.Registry {
			case DownstreamRegistry:
				summary.DownstreamImages++
			case TenantWorkspace:
				summary.TenantImages++
			}
		} else {
			summary.InaccessibleImages++
		}
	}

	return summary
}
