package proxy

import "fmt"

type nodeUnreachableError struct {
	nodeID string
	cause  error
}

func (err nodeUnreachableError) Error() string {
	return fmt.Sprintf("%s unreachable: %v", err.nodeName(), err.cause)
}

func (err nodeUnreachableError) Unwrap() error {
	return err.cause
}

func (err nodeUnreachableError) clientMessage() string {
	return err.nodeName() + " unavailable"
}

func (err nodeUnreachableError) nodeName() string {
	if err.nodeID == "" {
		return "remote node"
	}
	return "node " + err.nodeID
}

func (service *Service) nodeUnreachable(nodeID string, nodeURL string, cause error) error {
	if nodeID == "" {
		nodeID = service.nodeIDForURL(nodeURL)
	}
	return nodeUnreachableError{nodeID: nodeID, cause: cause}
}
