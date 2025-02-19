package ami

import (
	"context"
	"fmt"

	"github.com/weaveworks/eksctl/pkg/awsapi"

	"github.com/kris-nova/logger"

	api "github.com/weaveworks/eksctl/pkg/apis/eksctl.io/v1alpha5"
	instanceutils "github.com/weaveworks/eksctl/pkg/utils/instance"
)

const (
	// ownerIDUbuntuFamily is the owner ID used for Ubuntu AMIs
	ownerIDUbuntuFamily = "099720109477"

	// ownerIDWindowsFamily is the owner ID used for Ubuntu AMIs
	ownerIDWindowsFamily = "801119661308"
)

// MakeImageSearchPatterns creates a map of image search patterns by image OS family and class
func MakeImageSearchPatterns(version string) map[string]map[int]string {
	return map[string]map[int]string{
		api.NodeImageFamilyAmazonLinux2: {
			ImageClassGeneral: fmt.Sprintf("amazon-eks-node-%s-v*", version),
			ImageClassGPU:     fmt.Sprintf("amazon-eks-gpu-node-%s-*", version),
			ImageClassARM:     fmt.Sprintf("amazon-eks-arm64-node-%s-*", version),
		},
		api.NodeImageFamilyUbuntu2004: {
			ImageClassGeneral: fmt.Sprintf("ubuntu-eks/k8s_%s/images/*20.04-amd64*", version),
			ImageClassARM:     fmt.Sprintf("ubuntu-eks/k8s_%s/images/*20.04-arm64*", version),
		},
		api.NodeImageFamilyUbuntu1804: {
			ImageClassGeneral: fmt.Sprintf("ubuntu-eks/k8s_%s/images/*18.04*", version),
		},
		api.NodeImageFamilyWindowsServer2019CoreContainer: {
			ImageClassGeneral: fmt.Sprintf("Windows_Server-2019-English-Core-EKS_Optimized-%v-*", version),
		},
		api.NodeImageFamilyWindowsServer2019FullContainer: {
			ImageClassGeneral: fmt.Sprintf("Windows_Server-2019-English-Full-EKS_Optimized-%v-*", version),
		},
	}
}

// OwnerAccountID returns the AWS account ID that owns worker AMI.
func OwnerAccountID(imageFamily, region string) (string, error) {
	switch imageFamily {
	case api.NodeImageFamilyUbuntu2004, api.NodeImageFamilyUbuntu1804:
		return ownerIDUbuntuFamily, nil
	case api.NodeImageFamilyAmazonLinux2:
		return api.EKSResourceAccountID(region), nil
	default:
		if api.IsWindowsImage(imageFamily) {
			return ownerIDWindowsFamily, nil
		}
		return "", fmt.Errorf("unable to determine the account owner for image family %s", imageFamily)
	}
}

// AutoResolver resolves the AMi to the defaults for the region
// by querying AWS EC2 API for the AMI to use
type AutoResolver struct {
	api awsapi.EC2
}

// Resolve will return an AMI to use based on the default AMI for
// each region
func (r *AutoResolver) Resolve(ctx context.Context, region, version, instanceType, imageFamily string) (string, error) {
	logger.Debug("resolving AMI using AutoResolver for region %s, instanceType %s and imageFamily %s", region, instanceType, imageFamily)

	imageClasses := MakeImageSearchPatterns(version)[imageFamily]
	namePattern := imageClasses[ImageClassGeneral]
	if instanceutils.IsGPUInstanceType(instanceType) {
		var ok bool
		namePattern, ok = imageClasses[ImageClassGPU]
		if !ok {
			logger.Critical("image family %s doesn't support GPU image class", imageFamily)
			return "", NewErrFailedResolution(region, version, instanceType, imageFamily)
		}
	}

	if instanceutils.IsARMInstanceType(instanceType) {
		var ok bool
		namePattern, ok = imageClasses[ImageClassARM]
		if !ok {
			logger.Critical("image family %s doesn't support ARM image class", imageFamily)
			return "", NewErrFailedResolution(region, version, instanceType, imageFamily)
		}
	}

	ownerAccount, err := OwnerAccountID(imageFamily, region)
	if err != nil {
		logger.Critical("%v", err)
		return "", NewErrFailedResolution(region, version, instanceType, imageFamily)
	}

	id, err := FindImage(ctx, r.api, ownerAccount, namePattern)
	if err != nil {
		return "", fmt.Errorf("error getting AMI from EC2 API: %w. please verify that AMI Family is supported", err)
	}

	return id, nil
}
