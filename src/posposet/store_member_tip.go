package posposet

import "github.com/Fantom-foundation/go-lachesis/src/hash"

func (s *Store) SetLastEvent(from hash.Peer, id hash.Event) {
	key := from.Bytes()

	if err := s.epochTable.Tips.Put(key, id.Bytes()); err != nil {
		s.Fatal(err)
	}
}

func (s *Store) GetLastEvent(from hash.Peer) *hash.Event {
	key := from.Bytes()

	idBytes, err := s.epochTable.Tips.Get(key)
	if err != nil {
		s.Fatal(err)
	}
	if idBytes == nil {
		return nil
	}
	id := hash.BytesToEvent(idBytes)
	return &id
}
