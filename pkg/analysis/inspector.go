package analysis

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/types"
	"github.com/sirupsen/logrus"
)

// InspectImage performs inspection of a single image reference.
func InspectImage(ctx context.Context, imageRefStr string, stream string) (*ImageResult, error) {
	logrus.Debugf("InspectImage: %s (stream: %s)", imageRefStr, stream)

	imageRef, err := ParseImageRef(imageRefStr)
	if err != nil {
		return &ImageResult{
			Reference:  imageRefStr,
			Accessible: false,
			Registry:   NotAccessible,
			Error:      fmt.Sprintf("invalid image reference: %v", err),
		}, nil
	}

	logrus.Debugf("Parsed to: registry=%s repo=%s tag=%s digest=%s",
		imageRef.Registry, imageRef.Repo, imageRef.Tag, imageRef.Digest)

	result := &ImageResult{
		Reference: imageRefStr,
	}

	logrus.Debugf("Attempting to inspect: %s", imageRef.String())
	if info, err := inspectImageRef(ctx, imageRef); err == nil {
		result.Accessible = true
		if imageRef.Registry == "registry.redhat.io" {
			result.Registry = DownstreamRegistry
		} else if imageRef.Registry == "quay.io" && strings.Contains(imageRef.Repo, "redhat-user-workloads") {
			result.Registry = TenantWorkspace
		} else {
			result.Registry = DownstreamRegistry
		}
		result.Info = convertToImageInfo(info)
		logrus.Debugf("Successfully inspected %s", imageRef.String())
		return result, nil
	}

	logrus.Debugf("Primary inspection failed, attempting tenant workspace conversion")
	tenantRef, err := imageRef.ConvertToTenantWorkspace(stream)
	if err != nil {
		logrus.Debugf("Cannot convert to tenant workspace: %v", err)
		result.Accessible = false
		result.Registry = NotAccessible
		result.Error = "not accessible in any registry"
		return result, nil
	}

	logrus.Debugf("Attempting tenant workspace: %s", tenantRef.String())
	if info, err := inspectImageRef(ctx, tenantRef); err == nil {
		result.Accessible = true
		result.Registry = TenantWorkspace
		result.TenantRef = tenantRef.String()
		result.Info = convertToImageInfo(info)
		logrus.Debugf("Successfully inspected via tenant workspace: %s", tenantRef.String())
		return result, nil
	}

	logrus.Debugf("Failed to inspect image in any registry")
	result.Accessible = false
	result.Registry = NotAccessible
	result.Error = "not accessible in downstream or tenant registry"
	return result, nil
}

// inspectImageRef inspects a specific image reference and returns
// metadata.
func inspectImageRef(ctx context.Context, imageRef ImageRef) (*types.ImageInspectInfo, error) {
	logrus.Debugf("inspectImageRef: %s", imageRef.String())

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

	info, err := img.Inspect(ctx)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("Inspected %s successfully", imageRef.String())
	if info.Created != nil {
		logrus.Debugf("  Created: %s", info.Created)
	}
	if info.Labels != nil && info.Labels["version"] != "" {
		logrus.Debugf("  Version: %s", info.Labels["version"])
	}

	return info, nil
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

	if imageInfo.GitCommit != "" && imageInfo.GitURL != "" {
		if commitDate := fetchCommitDate(imageInfo.GitURL, imageInfo.GitCommit); commitDate != nil {
			imageInfo.CommitDate = commitDate
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
