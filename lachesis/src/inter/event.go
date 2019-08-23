package inter

import (
	"fmt"

	"github.com/golang/protobuf/proto"

	"github.com/Fantom-foundation/go-lachesis/lachesis/src/crypto"
	"github.com/Fantom-foundation/go-lachesis/lachesis/src/cryptoaddr"
	"github.com/Fantom-foundation/go-lachesis/lachesis/src/hash"
	"github.com/Fantom-foundation/go-lachesis/lachesis/src/inter/idx"
	"github.com/Fantom-foundation/go-lachesis/lachesis/src/inter/wire"
)

// Event is a poset event.
type Event struct {
	SfNum                idx.SuperFrame
	Seq                  idx.Event
	Creator              hash.Peer
	SelfParent           hash.Event
	Parents              hash.Events
	LamportTime          Timestamp
	InternalTransactions []*InternalTransaction
	ExternalTransactions ExtTxns
	Sign                 []byte

	hash hash.Event // cache for .Hash()
}

// SignBy signs event by private key.
func (e *Event) SignBy(priv *crypto.PrivateKey) error {
	eventHash := e.Hash()

	sig, err := priv.Sign(eventHash.Bytes())
	if err != nil {
		return err
	}

	e.Sign = sig
	return nil
}

// Verify sign event by public key.
func (e *Event) VerifySignature() bool {
	return cryptoaddr.VerifySignature(e.Creator, hash.Hash(e.Hash()), e.Sign)
}

// Hash calcs hash of event.
func (e *Event) Hash() hash.Event {
	if e.hash.IsZero() {
		e.hash = EventHashOf(e)
	}
	return e.hash
}

// FindInternalTxn find transaction in event's internal transactions list.
// TODO: use map
func (e *Event) FindInternalTxn(idx hash.Transaction) *InternalTransaction {
	for _, txn := range e.InternalTransactions {
		if TransactionHashOf(e.Creator, txn.Nonce) == idx {
			return txn
		}
	}
	return nil
}

// String returns string representation.
func (e *Event) String() string {
	return fmt.Sprintf("Event{%s, %s, t=%d}", e.Hash().String(), e.Parents.String(), e.LamportTime)
}

// ToWire converts to proto.Message.
func (e *Event) ToWire() (*wire.Event, *wire.Event_ExtTxnsValue) {
	if e == nil {
		return nil, nil
	}

	extTxns, extTxnsHash := e.ExternalTransactions.ToWire()

	return &wire.Event{
		SfNum:                uint64(e.SfNum),
		Seq:                  uint64(e.Seq),
		Creator:              e.Creator.Hex(),
		Parents:              e.Parents.ToWire(e.SelfParent),
		LamportTime:          uint64(e.LamportTime),
		InternalTransactions: InternalTransactionsToWire(e.InternalTransactions),
		ExternalTransactions: extTxnsHash,
		Sign:                 e.Sign,
	}, extTxns
}

// WireToEvent converts from wire.
func WireToEvent(w *wire.Event) *Event {
	if w == nil {
		return nil
	}
	self, all := hash.WireToEventHashes(w.Parents)
	return &Event{
		SfNum:                idx.SuperFrame(w.SfNum),
		Seq:                  idx.Event(w.Seq),
		Creator:              hash.HexToPeer(w.Creator),
		SelfParent:           self,
		Parents:              all,
		LamportTime:          Timestamp(w.LamportTime),
		InternalTransactions: WireToInternalTransactions(w.InternalTransactions),
		ExternalTransactions: WireToExtTxns(w),
		Sign:                 w.Sign,
	}
}

/*
 * Utils:
 */

// EventHashOf calcs hash of event.
func EventHashOf(e *Event) hash.Event {
	w, _ := e.ToWire()
	w.Sign = []byte{}

	buf, err := proto.Marshal(w)
	if err != nil {
		log.Fatal(err)
	}

	return hash.Event(hash.Of(buf))
}

// FakeFuzzingEvents generates random independent events for test purpose.
func FakeFuzzingEvents() (res []*Event) {
	creators := []hash.Peer{
		{},
		hash.FakePeer(),
		hash.FakePeer(),
		hash.FakePeer(),
	}
	parents := []hash.Events{
		hash.FakeEvents(1),
		hash.FakeEvents(2),
		hash.FakeEvents(8),
	}
	extTxns := [][][]byte{
		nil,
		[][]byte{
			[]byte("fake external transaction 1"),
			[]byte("fake external transaction 2"),
		},
	}
	i := 0
	for c := 0; c < len(creators); c++ {
		for p := 0; p < len(parents); p++ {
			e := &Event{
				Seq:     idx.Event(p),
				Creator: creators[c],
				Parents: parents[p],
				InternalTransactions: []*InternalTransaction{
					{
						Amount:   999,
						Receiver: creators[c],
					},
				},
				ExternalTransactions: ExtTxns{
					Value: extTxns[i%len(extTxns)],
				},
			}

			for p := range e.Parents {
				e.SelfParent = p
				break
			}

			res = append(res, e)
			i++
		}
	}
	return
}
