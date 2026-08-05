package sync

import (
	"sort"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func TestRoleManager_New(t *testing.T) {
	id := peer.ID("self")
	rm := NewRoleManager(id)
	assert.NotNil(t, rm)
	assert.Equal(t, RoleFollower, rm.GetRole())
	assert.False(t, rm.IsLeader())
	assert.True(t, rm.CanWrite())
}

func TestRoleManager_ElectLeader_SingleNode(t *testing.T) {
	id := peer.ID("alpha")
	rm := NewRoleManager(id)

	role := rm.ElectLeader()
	assert.Equal(t, RoleLeader, role)
	assert.True(t, rm.IsLeader())
	assert.True(t, rm.CanWrite())
}

func TestRoleManager_ElectLeader_LowestID(t *testing.T) {
	self := peer.ID("beta")
	peerA := peer.ID("alpha")
	peerC := peer.ID("charlie")

	rm := NewRoleManager(self)
	rm.UpdatePeer(peerA, &PeerInfo{ID: peerA})
	rm.UpdatePeer(peerC, &PeerInfo{ID: peerC})

	role := rm.ElectLeader()
	assert.Equal(t, RoleFollower, role)
	assert.False(t, rm.IsLeader())
}

func TestRoleManager_ElectLeader_SelfLowest(t *testing.T) {
	self := peer.ID("alpha")
	peerB := peer.ID("beta")

	rm := NewRoleManager(self)
	rm.UpdatePeer(peerB, &PeerInfo{ID: peerB})

	assert.Equal(t, RoleLeader, rm.ElectLeader())
	assert.True(t, rm.IsLeader())
}

func TestRoleManager_SetReadonly(t *testing.T) {
	rm := NewRoleManager(peer.ID("self"))
	rm.SetReadonly()
	assert.Equal(t, RoleReadonly, rm.GetRole())
	assert.False(t, rm.IsLeader())
	assert.False(t, rm.CanWrite())
}

func TestRoleManager_LexicographicOrder(t *testing.T) {
	ids := []string{"node-10", "node-2", "node-1"}
	sort.Strings(ids)
	assert.Equal(t, []string{"node-1", "node-10", "node-2"}, ids)
}
