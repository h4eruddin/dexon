package dex

import (
	"crypto/ecdsa"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/dexon-foundation/dexon/crypto"
	"github.com/dexon-foundation/dexon/p2p/enode"
)

func TestPeerSetBuildAndForgetConn(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	server := newTestP2PServer(key)
	self := server.Self()
	table := newNodeTable()

	gov := &testGovernance{
		numChainsFunc: func(uint64) uint32 {
			return 3
		},
	}

	var nodes []*enode.Node
	for i := 0; i < 9; i++ {
		nodes = append(nodes, randomV4CompactNode())
	}

	round10 := [][]*enode.Node{
		{self, nodes[1], nodes[2]},
		{nodes[1], nodes[3]},
		{nodes[2], nodes[4]},
	}
	round11 := [][]*enode.Node{
		{self, nodes[1], nodes[5]},
		{nodes[5], nodes[6]},
		{self, nodes[2], nodes[4]},
	}
	round12 := [][]*enode.Node{
		{self, nodes[3], nodes[5]},
		{self, nodes[7], nodes[8]},
		{self, nodes[2], nodes[6]},
	}

	gov.notarySetFunc = func(
		round uint64, cid uint32) (map[string]struct{}, error) {
		m := map[uint64][][]*enode.Node{
			10: round10,
			11: round11,
			12: round12,
		}
		return newTestNodeSet(m[round][cid]), nil
	}

	gov.dkgSetFunc = func(round uint64) (map[string]struct{}, error) {
		m := map[uint64][]*enode.Node{
			10: {self, nodes[1], nodes[3]},
			11: {nodes[1], nodes[2], nodes[5]},
			12: {self, nodes[3], nodes[5]},
		}
		return newTestNodeSet(m[round]), nil
	}

	ps := newPeerSet(gov, server, table)

	// build round 10
	ps.BuildConnection(10)
	ps.BuildConnection(11)
	ps.BuildConnection(12)

	expectedlabel2Nodes := map[peerLabel]map[string]*enode.Node{
		{set: notaryset, round: 10, chainID: 0}: {
			self.ID().String():     self,
			nodes[1].ID().String(): nodes[1],
			nodes[2].ID().String(): nodes[2],
		},
		{set: notaryset, round: 10, chainID: 1}: {
			nodes[1].ID().String(): nodes[1],
			nodes[3].ID().String(): nodes[3],
		},
		{set: notaryset, round: 10, chainID: 2}: {
			nodes[2].ID().String(): nodes[2],
			nodes[4].ID().String(): nodes[4],
		},
		{set: dkgset, round: 10}: {
			self.ID().String():     self,
			nodes[1].ID().String(): nodes[1],
			nodes[3].ID().String(): nodes[3],
		},
		{set: notaryset, round: 11, chainID: 0}: {
			self.ID().String():     self,
			nodes[1].ID().String(): nodes[1],
			nodes[5].ID().String(): nodes[5],
		},
		{set: notaryset, round: 11, chainID: 1}: {
			nodes[5].ID().String(): nodes[5],
			nodes[6].ID().String(): nodes[6],
		},
		{set: notaryset, round: 11, chainID: 2}: {
			self.ID().String():     self,
			nodes[2].ID().String(): nodes[2],
			nodes[4].ID().String(): nodes[4],
		},
		{set: dkgset, round: 11}: {
			nodes[1].ID().String(): nodes[1],
			nodes[2].ID().String(): nodes[2],
			nodes[5].ID().String(): nodes[5],
		},
		{set: notaryset, round: 12, chainID: 0}: {
			self.ID().String():     self,
			nodes[3].ID().String(): nodes[3],
			nodes[5].ID().String(): nodes[5],
		},
		{set: notaryset, round: 12, chainID: 1}: {
			self.ID().String():     self,
			nodes[7].ID().String(): nodes[7],
			nodes[8].ID().String(): nodes[8],
		},
		{set: notaryset, round: 12, chainID: 2}: {
			self.ID().String():     self,
			nodes[2].ID().String(): nodes[2],
			nodes[6].ID().String(): nodes[6],
		},
		{set: dkgset, round: 12}: {
			self.ID().String():     self,
			nodes[3].ID().String(): nodes[3],
			nodes[5].ID().String(): nodes[5],
		},
	}

	if !reflect.DeepEqual(ps.label2Nodes, expectedlabel2Nodes) {
		t.Errorf("label2Nodes not match")
	}

	expectedDirectConn := map[peerLabel]struct{}{
		{set: notaryset, round: 10, chainID: 0}: {},
		{set: notaryset, round: 11, chainID: 0}: {},
		{set: notaryset, round: 11, chainID: 2}: {},
		{set: notaryset, round: 12, chainID: 0}: {},
		{set: notaryset, round: 12, chainID: 1}: {},
		{set: notaryset, round: 12, chainID: 2}: {},
		{set: dkgset, round: 10}:                {},
		{set: dkgset, round: 12}:                {},
	}

	if !reflect.DeepEqual(ps.directConn, expectedDirectConn) {
		t.Errorf("direct conn not match")
	}

	expectedGroupConn := []peerLabel{
		{set: notaryset, round: 10, chainID: 1},
		{set: notaryset, round: 10, chainID: 2},
		{set: notaryset, round: 11, chainID: 1},
		{set: dkgset, round: 11},
	}

	if len(ps.groupConnPeers) != len(expectedGroupConn) {
		t.Errorf("group conn peers not match")
	}

	for _, l := range expectedGroupConn {
		if len(ps.groupConnPeers[l]) == 0 {
			t.Errorf("group conn peers is 0")
		}
	}

	expectedAllDirect := make(map[string]map[peerLabel]struct{})

	for l := range ps.directConn {
		for id := range ps.label2Nodes[l] {
			if expectedAllDirect[id] == nil {
				expectedAllDirect[id] = make(map[peerLabel]struct{})
			}
			expectedAllDirect[id][l] = struct{}{}
		}
	}

	for l, peers := range ps.groupConnPeers {
		for id := range peers {
			if expectedAllDirect[id] == nil {
				expectedAllDirect[id] = make(map[peerLabel]struct{})
			}
			expectedAllDirect[id][l] = struct{}{}
		}
	}

	if !reflect.DeepEqual(ps.allDirectPeers, expectedAllDirect) {
		t.Errorf("all direct peers not match")
	}

	// forget round 11
	ps.ForgetConnection(11)

	expectedlabel2Nodes = map[peerLabel]map[string]*enode.Node{
		{set: notaryset, round: 12, chainID: 0}: {
			self.ID().String():     self,
			nodes[3].ID().String(): nodes[3],
			nodes[5].ID().String(): nodes[5],
		},
		{set: notaryset, round: 12, chainID: 1}: {
			self.ID().String():     self,
			nodes[7].ID().String(): nodes[7],
			nodes[8].ID().String(): nodes[8],
		},
		{set: notaryset, round: 12, chainID: 2}: {
			self.ID().String():     self,
			nodes[2].ID().String(): nodes[2],
			nodes[6].ID().String(): nodes[6],
		},
		{set: dkgset, round: 12}: {
			self.ID().String():     self,
			nodes[3].ID().String(): nodes[3],
			nodes[5].ID().String(): nodes[5],
		},
	}

	if !reflect.DeepEqual(ps.label2Nodes, expectedlabel2Nodes) {
		t.Errorf("label2Nodes not match")
	}

	expectedDirectConn = map[peerLabel]struct{}{
		{set: notaryset, round: 12, chainID: 0}: {},
		{set: notaryset, round: 12, chainID: 1}: {},
		{set: notaryset, round: 12, chainID: 2}: {},
		{set: dkgset, round: 12}:                {},
	}

	if !reflect.DeepEqual(ps.directConn, expectedDirectConn) {
		t.Error("direct conn not match")
	}

	expectedGroupConn = []peerLabel{}

	if len(ps.groupConnPeers) != len(expectedGroupConn) {
		t.Errorf("group conn peers not match")
	}

	for _, l := range expectedGroupConn {
		if len(ps.groupConnPeers[l]) == 0 {
			t.Errorf("group conn peers is 0")
		}
	}

	expectedAllDirect = make(map[string]map[peerLabel]struct{})

	for l := range ps.directConn {
		for id := range ps.label2Nodes[l] {
			if expectedAllDirect[id] == nil {
				expectedAllDirect[id] = make(map[peerLabel]struct{})
			}
			expectedAllDirect[id][l] = struct{}{}
		}
	}

	for l, peers := range ps.groupConnPeers {
		for id := range peers {
			if expectedAllDirect[id] == nil {
				expectedAllDirect[id] = make(map[peerLabel]struct{})
			}
			expectedAllDirect[id][l] = struct{}{}
		}
	}

	if !reflect.DeepEqual(ps.allDirectPeers, expectedAllDirect) {
		t.Errorf("all direct peers not match")
	}

	// forget round 12
	ps.ForgetConnection(12)

	expectedlabel2Nodes = map[peerLabel]map[string]*enode.Node{}
	if !reflect.DeepEqual(ps.label2Nodes, expectedlabel2Nodes) {
		t.Errorf("label2Nodes not match")
	}

	expectedDirectConn = map[peerLabel]struct{}{}

	if !reflect.DeepEqual(ps.directConn, expectedDirectConn) {
		t.Error("direct conn not match")
	}

	expectedGroupConn = []peerLabel{}

	if len(ps.groupConnPeers) != len(expectedGroupConn) {
		t.Errorf("group conn peers not match")
	}

	for _, l := range expectedGroupConn {
		if len(ps.groupConnPeers[l]) == 0 {
			t.Errorf("group conn peers is 0")
		}
	}

	expectedAllDirect = make(map[string]map[peerLabel]struct{})

	for l := range ps.directConn {
		for id := range ps.label2Nodes[l] {
			if expectedAllDirect[id] == nil {
				expectedAllDirect[id] = make(map[peerLabel]struct{})
			}
			expectedAllDirect[id][l] = struct{}{}
		}
	}

	for l, peers := range ps.groupConnPeers {
		for id := range peers {
			if expectedAllDirect[id] == nil {
				expectedAllDirect[id] = make(map[peerLabel]struct{})
			}
			expectedAllDirect[id][l] = struct{}{}
		}
	}

	if !reflect.DeepEqual(ps.allDirectPeers, expectedAllDirect) {
		t.Errorf("all direct peers not match")
	}
}

func newTestNodeSet(nodes []*enode.Node) map[string]struct{} {
	m := make(map[string]struct{})
	for _, node := range nodes {
		b := crypto.FromECDSAPub(node.Pubkey())
		m[hex.EncodeToString(b)] = struct{}{}
	}
	return m
}

func randomV4CompactNode() *enode.Node {
	var err error
	var privkey *ecdsa.PrivateKey
	for {
		privkey, err = crypto.GenerateKey()
		if err == nil {
			break
		}
	}
	return enode.NewV4(&privkey.PublicKey, nil, 0, 0)
}