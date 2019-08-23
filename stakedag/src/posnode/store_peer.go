package posnode

import (
	"github.com/golang/protobuf/proto"

	"github.com/Fantom-foundation/go-lachesis/src/hash"
	"github.com/Fantom-foundation/go-lachesis/src/inter/idx"
	"github.com/Fantom-foundation/go-lachesis/stakedag/src/posnode/api"
)

// BootstrapPeers stores peer list.
func (s *Store) BootstrapPeers(peers ...*Peer) {
	if len(peers) < 1 {
		return
	}

	// save peers
	batch := s.table.Peers.NewBatch()
	defer batch.Reset()

	ids := make([]hash.Peer, 0, len(peers))
	for _, peer := range peers {
		// skip empty
		if peer == nil || peer.ID.IsEmpty() || peer.Host == "" {
			continue
		}

		var pbf proto.Buffer
		w := peer.ToWire()
		if err := pbf.Marshal(w); err != nil {
			s.Fatal(err)
		}
		if err := batch.Put(peer.ID.Bytes(), pbf.Bytes()); err != nil {
			s.Fatal(err)
		}
		ids = append(ids, peer.ID)
	}

	if err := batch.Write(); err != nil {
		s.Fatal(err)
	}

	s.SetTopPeers(ids)
}

// SetPeer stores peer.
func (s *Store) SetPeer(peer *Peer) {
	info := peer.ToWire()
	s.SetWirePeer(peer.ID, info)
}

// GetPeer returns stored peer.
func (s *Store) GetPeer(id hash.Peer) *Peer {
	w := s.GetWirePeer(id)
	return WireToPeer(w)
}

// SetWirePeer stores peer info.
func (s *Store) SetWirePeer(id hash.Peer, info *api.PeerInfo) {
	s.set(s.table.Peers, id.Bytes(), info)
}

// GetWirePeer returns stored peer info.
// Result is a ready gRPC message.
func (s *Store) GetWirePeer(id hash.Peer) *api.PeerInfo {
	w, _ := s.get(s.table.Peers, id.Bytes(), &api.PeerInfo{}).(*api.PeerInfo)
	return w
}

// SetTopPeers stores peers.top.
func (s *Store) SetTopPeers(ids []hash.Peer) {
	var key = []byte("current")
	w := IDsToWire(ids)
	s.set(s.table.PeersTop, key, w)
}

// GetTopPeers returns peers.top.
func (s *Store) GetTopPeers() []hash.Peer {
	var key = []byte("current")
	w, _ := s.get(s.table.PeersTop, key, &api.PeerIDs{}).(*api.PeerIDs)
	return WireToIDs(w)
}

// SetPeerHeight stores last event index of peer.
func (s *Store) SetPeerHeight(id hash.Peer, sf idx.SuperFrame, height idx.Event) {
	key := append(sf.Bytes(), id.Bytes()...)

	if err := s.table.PeerHeights.Put(key, height.Bytes()); err != nil {
		s.Fatal(err)
	}
}

// GetPeerHeight returns last event index of peer.
func (s *Store) GetPeerHeight(id hash.Peer, sf idx.SuperFrame) idx.Event {
	key := append(sf.Bytes(), id.Bytes()...)

	buf, err := s.table.PeerHeights.Get(key)
	if err != nil {
		s.Fatal(err)
	}
	if buf == nil {
		return 0
	}

	return idx.BytesToEvent(buf)
}

// GetAllPeerHeight returns last event index of all peers.
func (s *Store) GetAllPeerHeight(sf idx.SuperFrame) map[hash.Peer]idx.Event {
	res := make(map[hash.Peer]idx.Event)

	prefix := sf.Bytes()
	err := s.table.PeerHeights.ForEach(prefix, func(k, v []byte) bool {
		peer := hash.BytesToPeer(k[len(prefix):])
		height := idx.BytesToEvent(v)
		res[peer] = height
		return true
	})
	if err != nil {
		s.Fatal(err)
	}

	return res
}
