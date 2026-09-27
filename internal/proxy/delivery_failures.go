package proxy

import "sync"

type deliveryFailures struct {
	mu   sync.Mutex
	last map[string]string
}

func newDeliveryFailures() *deliveryFailures {
	return &deliveryFailures{last: map[string]string{}}
}

func (failures *deliveryFailures) outcomeChanged(nodeID string, err error) bool {
	outcome := ""
	if err != nil {
		outcome = err.Error()
	}
	failures.mu.Lock()
	defer failures.mu.Unlock()
	if failures.last[nodeID] == outcome {
		return false
	}
	failures.last[nodeID] = outcome
	return true
}
