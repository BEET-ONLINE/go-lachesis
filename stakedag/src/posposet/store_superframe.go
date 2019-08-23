package posposet

import (
	"github.com/Fantom-foundation/go-lachesis/src/inter/idx"
	"github.com/Fantom-foundation/go-lachesis/stakedag/src/posposet/wire"
)

// SetSuperFrame stores super-frame.
func (s *Store) SetSuperFrame(n idx.SuperFrame, sf *superFrame) {
	s.set(s.table.SuperFrames, n.Bytes(), sf.ToWire())
}

// GetSuperFrame returns stored super-frame.
func (s *Store) GetSuperFrame(n idx.SuperFrame) *superFrame {
	w, exists := s.get(s.table.SuperFrames, n.Bytes(), &wire.SuperFrame{}).(*wire.SuperFrame)
	if !exists {
		return nil
	}
	return wireToSuperFrame(w)
}
