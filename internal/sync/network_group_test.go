package sync

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func TestNewNetworkGroup(t *testing.T) {
	ng := NewNetworkGroup("group-1", "Test Group", "A test group")
	assert.NotNil(t, ng)
	assert.Equal(t, "group-1", ng.GroupID)
	assert.Equal(t, "Test Group", ng.Name)
	assert.Equal(t, "A test group", ng.Description)
	assert.Empty(t, ng.Members)
	assert.False(t, ng.CreatedAt.IsZero())
	assert.False(t, ng.UpdatedAt.IsZero())
}

func TestNetworkGroup_JoinAndLeave(t *testing.T) {
	ng := NewNetworkGroup("group-1", "Test", "")
	peerID := peer.ID("peer-1")

	err := ng.JoinGroup(peerID)
	assert.NoError(t, err)
	assert.True(t, ng.IsMember(peerID))
	assert.Len(t, ng.GetMembers(), 1)

	err = ng.JoinGroup(peerID)
	assert.Error(t, err)
	assert.Len(t, ng.GetMembers(), 1)

	ng.LeaveGroup(peerID)
	assert.False(t, ng.IsMember(peerID))
	assert.Empty(t, ng.GetMembers())
}

func TestNetworkGroup_GetMembers_Copy(t *testing.T) {
	ng := NewNetworkGroup("group-1", "Test", "")
	peerID := peer.ID("peer-1")
	ng.JoinGroup(peerID)

	members := ng.GetMembers()
	members[0] = "tampered"

	assert.Equal(t, peerID.String(), ng.GetMembers()[0])
}

func TestGroupManager_CreateAndGet(t *testing.T) {
	gm := NewGroupManager()
	group, err := gm.CreateGroup("group-1", "Test", "")
	assert.NoError(t, err)
	assert.NotNil(t, group)

	found, exists := gm.GetGroup("group-1")
	assert.True(t, exists)
	assert.Equal(t, group, found)

	_, err = gm.CreateGroup("group-1", "Test", "")
	assert.Error(t, err)
}

func TestGroupManager_DeleteGroup(t *testing.T) {
	gm := NewGroupManager()
	_, _ = gm.CreateGroup("group-1", "Test", "")

	err := gm.DeleteGroup("group-1")
	assert.NoError(t, err)

	_, exists := gm.GetGroup("group-1")
	assert.False(t, exists)

	err = gm.DeleteGroup("missing")
	assert.Error(t, err)
}

func TestGroupManager_JoinLeave(t *testing.T) {
	gm := NewGroupManager()
	_, _ = gm.CreateGroup("group-1", "Test", "")
	peerID := peer.ID("peer-1")

	err := gm.JoinGroup("group-1", peerID)
	assert.NoError(t, err)

	err = gm.JoinGroup("missing", peerID)
	assert.Error(t, err)

	err = gm.LeaveGroup("group-1", peerID)
	assert.NoError(t, err)
	assert.False(t, gm.GetGroup("group-1").IsMember(peerID))

	err = gm.LeaveGroup("missing", peerID)
	assert.Error(t, err)
}

func TestGroupManager_GetGroupsByPeer(t *testing.T) {
	gm := NewGroupManager()
	_, _ = gm.CreateGroup("group-1", "Test", "")
	_, _ = gm.CreateGroup("group-2", "Test", "")
	_, _ = gm.CreateGroup("group-3", "Test", "")

	peerID := peer.ID("peer-1")
	_ = gm.JoinGroup("group-1", peerID)
	_ = gm.JoinGroup("group-3", peerID)

	groups := gm.GetGroupsByPeer(peerID)
	assert.Len(t, groups, 2)
}

func TestGroupManager_GetGroupMembers(t *testing.T) {
	gm := NewGroupManager()
	_, _ = gm.CreateGroup("group-1", "Test", "")
	peerID := peer.ID("peer-1")
	_ = gm.JoinGroup("group-1", peerID)

	members, err := gm.GetGroupMembers("group-1")
	assert.NoError(t, err)
	assert.Equal(t, []string{peerID.String()}, members)

	_, err = gm.GetGroupMembers("missing")
	assert.Error(t, err)
}

func TestGroupManager_SerializeDeserializeGroup(t *testing.T) {
	gm := NewGroupManager()
	_, _ = gm.CreateGroup("group-1", "Test", "desc")

	data, err := gm.SerializeGroup("group-1")
	assert.NoError(t, err)
	assert.Contains(t, string(data), "group-1")

	group, err := gm.DeserializeGroup(data)
	assert.NoError(t, err)
	assert.Equal(t, "group-1", group.GroupID)
	assert.Equal(t, "Test", group.Name)

	_, err = gm.SerializeGroup("missing")
	assert.Error(t, err)
}

func TestGroupManager_ListGroups(t *testing.T) {
	gm := NewGroupManager()
	_, _ = gm.CreateGroup("group-1", "A", "")
	_, _ = gm.CreateGroup("group-2", "B", "")

	groups := gm.ListGroups()
	assert.Len(t, groups, 2)
}
