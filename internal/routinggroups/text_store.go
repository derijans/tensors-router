package routinggroups

import "context"

type TextMember struct {
	NodeID  string `json:"node_id"`
	ModelID string `json:"model_id"`
}

type TextGroup struct {
	ID      string       `json:"id"`
	Members []TextMember `json:"members"`
}

func (store *Store) TextGroup(ctx context.Context, member TextMember) (TextGroup, bool, error) {
	group, ok, err := store.laneGroup(ctx, textLaneTables, textMemberToLane(member))
	return textGroupFromLane(group), ok, err
}

func (store *Store) TextGroups(ctx context.Context) ([]TextGroup, error) {
	groups, err := store.laneGroups(ctx, textLaneTables)
	if err != nil {
		return nil, err
	}
	result := make([]TextGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, textGroupFromLane(group))
	}
	return result, nil
}

func (store *Store) SetTextGroup(ctx context.Context, anchor TextMember, members []TextMember) (TextGroup, error) {
	group, err := store.setLaneGroup(ctx, textLaneTables, textMemberToLane(anchor), textMembersToLane(members))
	return textGroupFromLane(group), err
}

func (store *Store) DeleteTextGroup(ctx context.Context, member TextMember) error {
	if store == nil {
		return nil
	}
	_, err := store.SetTextGroup(ctx, member, nil)
	return err
}

func textMemberToLane(member TextMember) laneMember {
	return laneMember{NodeID: member.NodeID, ModelID: member.ModelID}
}

func textMembersToLane(members []TextMember) []laneMember {
	result := make([]laneMember, 0, len(members))
	for _, member := range members {
		result = append(result, textMemberToLane(member))
	}
	return result
}

func textGroupFromLane(group laneGroup) TextGroup {
	if group.ID == "" {
		return TextGroup{}
	}
	members := make([]TextMember, 0, len(group.Members))
	for _, member := range group.Members {
		members = append(members, TextMember{NodeID: member.NodeID, ModelID: member.ModelID})
	}
	return TextGroup{ID: group.ID, Members: members}
}
