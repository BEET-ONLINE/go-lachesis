package posnode

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"

	"github.com/Fantom-foundation/go-lachesis/src/logger"
	"github.com/Fantom-foundation/go-lachesis/stakedag/src/posnode/api"
)

func TestClient(t *testing.T) {
	logger.SetTestMode(t)

	server := NewForTests("server.fake", nil, nil)
	server.StartService()
	defer server.Stop()
	peer := server.AsPeer()

	node := NewForTests("client.fake", nil, nil)
	node.initClient()
	defer node.Stop()

	ping := func(t *testing.T) (proto.Message, error) {
		client, free, fail, err := node.ConnectTo(peer)
		if !assert.NoError(t, err) {
			return nil, err
		}
		defer free()

		ctx, cancel := context.WithTimeout(context.Background(), node.conf.ClientTimeout)
		defer cancel()

		id, ctx := api.ServerPeerID(ctx)

		pong, err := client.GetPeerInfo(ctx, &api.PeerRequest{})
		if err != nil {
			fail(err)
		} else {
			assert.Equal(t, server.ID, *id)
		}

		return pong, err
	}

	t.Run("1st-connection", func(t *testing.T) {
		assertar := assert.New(t)

		pong, err := ping(t)
		_ = assertar.NoError(err) && assertar.NotNil(pong)
	})

	t.Run("2nd-connection", func(t *testing.T) {
		assertar := assert.New(t)

		pong, err := ping(t)
		_ = assertar.NoError(err) && assertar.NotNil(pong)
	})

	t.Run("Re-connection 1", func(t *testing.T) {
		assertar := assert.New(t)

		server.StopService()
		server.StartService()
		pong, err := ping(t)
		// both results (err or not) are acceptable here
		_ = assertar.NotEqual(err == nil, api.IsProtoEmpty(&pong), "inconsistent result")
	})

	t.Run("Re-connection 2", func(t *testing.T) {
		assertar := assert.New(t)

		pong, err := ping(t)
		_ = assertar.NoError(err) && assertar.NotNil(pong)
	})

	// TODO: test the all situations.
}
