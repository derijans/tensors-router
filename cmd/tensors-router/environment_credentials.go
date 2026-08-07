package main

import (
	"os"

	"tensors-router/internal/config"
	"tensors-router/internal/credential"
)

const (
	inferenceTokenEnvironment     = "TENSORS_ROUTER_INFERENCE_TOKEN"
	inferenceTokenFileEnvironment = "TENSORS_ROUTER_INFERENCE_TOKEN_FILE"
	adminTokenEnvironment         = "TENSORS_ROUTER_ADMIN_TOKEN"
	adminTokenFileEnvironment     = "TENSORS_ROUTER_ADMIN_TOKEN_FILE"
	clusterTokenEnvironment       = "TENSORS_ROUTER_CLUSTER_TOKEN"
	clusterTokenFileEnvironment   = "TENSORS_ROUTER_CLUSTER_TOKEN_FILE"
)

func environmentLoadOptions(securityProfile string) (config.LoadOptions, error) {
	inferenceKey, _, err := credential.Resolve(credential.Source{
		Role:      "inference",
		ValueName: inferenceTokenEnvironment,
		Value:     os.Getenv(inferenceTokenEnvironment),
		FileName:  inferenceTokenFileEnvironment,
		FilePath:  os.Getenv(inferenceTokenFileEnvironment),
	})
	if err != nil {
		return config.LoadOptions{}, err
	}
	adminKey, _, err := credential.Resolve(credential.Source{
		Role:      "admin",
		ValueName: adminTokenEnvironment,
		Value:     os.Getenv(adminTokenEnvironment),
		FileName:  adminTokenFileEnvironment,
		FilePath:  os.Getenv(adminTokenFileEnvironment),
	})
	if err != nil {
		return config.LoadOptions{}, err
	}
	clusterToken, _, err := credential.Resolve(credential.Source{
		Role:      "cluster",
		ValueName: clusterTokenEnvironment,
		Value:     os.Getenv(clusterTokenEnvironment),
		FileName:  clusterTokenFileEnvironment,
		FilePath:  os.Getenv(clusterTokenFileEnvironment),
	})
	if err != nil {
		return config.LoadOptions{}, err
	}
	return config.LoadOptions{
		SecurityProfile: securityProfile,
		InferenceKey:    inferenceKey,
		AdminKey:        adminKey,
		ClusterToken:    clusterToken,
	}, nil
}
