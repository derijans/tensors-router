package routinggroups

import (
	"context"
	"database/sql"
)

// Member identifies one image model on one node. Members are declared by an
// operator and are not required to share a name, a config hash, or even a
// checkpoint with each other.
type Member struct {
	NodeID  string `json:"node_id"`
	ImageID string `json:"image_id"`
}

type Group struct {
	ID      string   `json:"id"`
	Members []Member `json:"members"`
}

type Store struct {
	writer *sql.DB
	reader *sql.DB
}

func NewStore(writer *sql.DB, reader *sql.DB) *Store {
	return &Store{writer: writer, reader: reader}
}

func (store *Store) Group(ctx context.Context, member Member) (Group, bool, error) {
	group, ok, err := store.laneGroup(ctx, imageLaneTables, memberToLane(member))
	return groupFromLane(group), ok, err
}

func (store *Store) Groups(ctx context.Context) ([]Group, error) {
	groups, err := store.laneGroups(ctx, imageLaneTables)
	if err != nil {
		return nil, err
	}
	result := make([]Group, 0, len(groups))
	for _, group := range groups {
		result = append(result, groupFromLane(group))
	}
	return result, nil
}

// SetGroup replaces whatever group the anchor belonged to with exactly the anchor
// plus the supplied members. See setLaneGroup for the shared invariants.
func (store *Store) SetGroup(ctx context.Context, anchor Member, members []Member) (Group, error) {
	group, err := store.setLaneGroup(ctx, imageLaneTables, memberToLane(anchor), membersToLane(members))
	return groupFromLane(group), err
}

func (store *Store) DeleteGroup(ctx context.Context, member Member) error {
	if store == nil {
		return nil
	}
	_, err := store.SetGroup(ctx, member, nil)
	return err
}

func memberToLane(member Member) laneMember {
	return laneMember{NodeID: member.NodeID, ModelID: member.ImageID}
}

func membersToLane(members []Member) []laneMember {
	result := make([]laneMember, 0, len(members))
	for _, member := range members {
		result = append(result, memberToLane(member))
	}
	return result
}

func groupFromLane(group laneGroup) Group {
	if group.ID == "" {
		return Group{}
	}
	members := make([]Member, 0, len(group.Members))
	for _, member := range group.Members {
		members = append(members, Member{NodeID: member.NodeID, ImageID: member.ModelID})
	}
	return Group{ID: group.ID, Members: members}
}
