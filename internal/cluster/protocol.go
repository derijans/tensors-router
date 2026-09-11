package cluster

import "fmt"

// ProtocolVersion is what this build speaks on the node-sync wire (snapshots,
// registration, runtime status). MinimumProtocolVersion is the oldest peer this
// build still understands. A release that only adds fields raises ProtocolVersion
// and leaves the minimum alone, so a rolling upgrade keeps working; a release that
// changes a shape raises both, so an old peer is refused rather than silently
// mis-decoded.
const ProtocolVersion = 2
const MinimumProtocolVersion = 2

// CompatiblePeer reports whether a peer advertising peerVersion satisfies this
// build's MinimumProtocolVersion. A peer that predates protocol versioning
// entirely sends the zero value, which falls below every floor and is refused by
// the same rule as a genuinely old build.
func CompatiblePeer(peerVersion int) error {
	if peerVersion < MinimumProtocolVersion {
		return fmt.Errorf("peer protocol version %d is below the minimum %d this build requires; update the peer", peerVersion, MinimumProtocolVersion)
	}
	return nil
}
