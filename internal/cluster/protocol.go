package cluster

import "fmt"

const ProtocolVersion = 2
const MinimumProtocolVersion = 2

func CompatiblePeer(peerVersion int) error {
	if peerVersion < MinimumProtocolVersion {
		return fmt.Errorf("peer protocol version %d is below the minimum %d this build requires; update the peer", peerVersion, MinimumProtocolVersion)
	}
	return nil
}
