package cryptoaddr

import (
	"github.com/btcsuite/btcd/btcec"

	"github.com/Fantom-foundation/go-lachesis/lachesis/src/crypto"
	"github.com/Fantom-foundation/go-lachesis/lachesis/src/hash"
)

// AddressOf calculates hash of the PublicKey.
func AddressOf(pk *crypto.PublicKey) hash.Peer {
	bytes := (*btcec.PublicKey)(pk).SerializeUncompressed()
	return hash.BytesToPeer(crypto.Keccak256(bytes))
}
