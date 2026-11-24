package proxy

import (
	"context"

	"tensors-router/internal/cluster"
)

func (service *Service) clusterMaster() bool {
	return service.clusterRole == cluster.RoleMaster
}

func (service *Service) exchangeWithNode(ctx context.Context, method string, nodeURL string, path string, requestJSON []byte) (cluster.RawResponse, error) {
	return service.clusterClient.Exchange(ctx, method, nodeURL, path, requestJSON)
}
