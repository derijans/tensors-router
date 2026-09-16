package proxy

import (
	"tensors-router/internal/cluster"
	"tensors-router/internal/routinggroups"
)

type routingLinkSnapshot struct {
	Image []routinggroups.Link `json:"image"`
	Text  []routinggroups.Link `json:"text"`
}

func (snapshot routingLinkSnapshot) linksFor(lane string) []routinggroups.Link {
	if lane == cluster.RouteLaneText {
		return snapshot.Text
	}
	return snapshot.Image
}

type laneLinkIndex struct {
	linked      map[routinggroups.Endpoint]bool
	lendTargets map[routinggroups.Endpoint][]routinggroups.Link
}

func newLaneLinkIndex(links []routinggroups.Link) laneLinkIndex {
	index := laneLinkIndex{
		linked:      map[routinggroups.Endpoint]bool{},
		lendTargets: map[routinggroups.Endpoint][]routinggroups.Link{},
	}
	for _, link := range links {
		index.linked[link.Owner] = true
		index.linked[link.Helper] = true
		index.lendTargets[link.Owner] = append(index.lendTargets[link.Owner], link)
	}
	return index
}

type routingLinkIndex struct {
	snapshot routingLinkSnapshot
	image    laneLinkIndex
	text     laneLinkIndex
}

func newRoutingLinkIndex(snapshot routingLinkSnapshot) *routingLinkIndex {
	return &routingLinkIndex{
		snapshot: snapshot,
		image:    newLaneLinkIndex(snapshot.Image),
		text:     newLaneLinkIndex(snapshot.Text),
	}
}

func (index *routingLinkIndex) lane(lane string) laneLinkIndex {
	if lane == cluster.RouteLaneText {
		return index.text
	}
	return index.image
}

func (index *routingLinkIndex) isLinked(lane string, endpoint routinggroups.Endpoint) bool {
	if index == nil {
		return false
	}
	return index.lane(lane).linked[endpoint]
}

func (index *routingLinkIndex) lendTargets(lane string, owner routinggroups.Endpoint) []routinggroups.Link {
	if index == nil {
		return nil
	}
	return index.lane(lane).lendTargets[owner]
}

func (index *routingLinkIndex) owners(lane string) []routinggroups.Endpoint {
	if index == nil {
		return nil
	}
	seen := map[routinggroups.Endpoint]bool{}
	var owners []routinggroups.Endpoint
	for _, link := range index.snapshot.linksFor(lane) {
		if !seen[link.Owner] {
			seen[link.Owner] = true
			owners = append(owners, link.Owner)
		}
	}
	return owners
}
