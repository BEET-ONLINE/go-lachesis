package node

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Fantom-foundation/go-lachesis/src/peers"
)

func TestSmartSelectorEmpty(t *testing.T) {
	assertO := assert.New(t)

	fp := fakePeers(0)

	ss := NewSmartPeerSelector(
		fp,
		"",
		func() (map[string]int64, error) {
			return nil, nil
		},
	)

	assertO.Nil(ss.Next(1))
}

func TestSmartSelectorLocalAddrOnly(t *testing.T) {
	assertO := assert.New(t)

	fp := fakePeers(1)
	fps := fp.ToPeerSlice()

	ss := NewSmartPeerSelector(
		fp,
		fps[0].NetAddr,
		func() (map[string]int64, error) {
			return nil, nil
		},
	)

	assertO.Nil(ss.Next(1))
}

func TestSmartSelectorUsed(t *testing.T) {
	assertO := assert.New(t)

	fp := fakePeers(3)
	fps := fp.ToPeerSlice()

	ss := NewSmartPeerSelector(
		fp,
		fps[0].NetAddr,
		func() (map[string]int64, error) {
			return nil, nil
		},
	)

	assertO.Equal(fps[1].NetAddr, ss.Next(1)[0].NetAddr)
	assertO.Equal(fps[1].NetAddr, ss.Next(1)[0].NetAddr)
	assertO.Equal(fps[1].NetAddr, ss.Next(1)[0].NetAddr)
}

func TestSmartSelectorFlagged(t *testing.T) {
	assertO := assert.New(t)

	fp := fakePeers(3)
	fps := fp.ToPeerSlice()

	ss := NewSmartPeerSelector(
		fp,
		fps[0].NetAddr,
		func() (map[string]int64, error) {
			return map[string]int64{
				fps[2].PubKeyHex: 1,
			}, nil
		},
	)

	assertO.Equal(fps[1].NetAddr, ss.Next(1)[0].NetAddr)
	assertO.Equal(fps[1].NetAddr, ss.Next(1)[0].NetAddr)
	assertO.Equal(fps[1].NetAddr, ss.Next(1)[0].NetAddr)
}

func TestSmartSelectorGeneral(t *testing.T) {
	assertO := assert.New(t)

	fp := fakePeers(4)
	fps := fp.ToPeerSlice()

	ss := NewSmartPeerSelector(
		fp,
		fps[3].NetAddr,
		func() (map[string]int64, error) {
			return map[string]int64{
				fps[0].PubKeyHex: 0,
				fps[1].PubKeyHex: 0,
				fps[2].PubKeyHex: 1,
				fps[3].PubKeyHex: 0,
			}, nil
		},
	)

	addresses := []string{fps[0].NetAddr, fps[1].NetAddr}
	assertO.Contains(addresses, ss.Next(1)[0].NetAddr)
	assertO.Contains(addresses, ss.Next(1)[0].NetAddr)
	assertO.Contains(addresses, ss.Next(1)[0].NetAddr)
	assertO.Contains(addresses, ss.Next(1)[0].NetAddr)
}

/*
 * go test -bench "BenchmarkSmartSelectorNext" -benchmem -run "^$" ./src/node
 */

func BenchmarkSmartSelectorNext(b *testing.B) {
	const fakePeersCount = 50

	participants1 := fakePeers(fakePeersCount)
	participants2 := clonePeers(participants1)

	flagTable1 := fakeFlagTable(participants1)

	ss1 := NewSmartPeerSelector(
		participants1,
		fakeAddr(0),
		func() (map[string]int64, error) {
			return flagTable1, nil
		},
	)
	rnd := NewRandomPeerSelector(
		participants2,
		fakeAddr(0),
	)

	b.ResetTimer()

	b.Run("smart Next()", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := ss1.Next(1)
			if p == nil {
				b.Fatal("No next peer")
				break
			}
			ss1.UpdateLast(p)
		}
	})

	b.Run("simple Next()", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := rnd.Next(1)
			if p == nil {
				b.Fatal("No next peer")
				break
			}
			rnd.UpdateLast(p)
		}
	})

}

/*
 * utility function for peer_selector2_test
 */

func fakeFlagTable(participants *peers.Peers) map[string]int64 {
	res := make(map[string]int64, participants.Len())
	for _, p := range participants.ToPeerSlice() {
		res[p.PubKeyHex] = rand.Int63n(2)
	}
	return res
}
