package gossip

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/quan8/go-ethereum/accounts"
	"github.com/quan8/go-ethereum/common"
	"github.com/quan8/go-ethereum/core/types"
	notify "github.com/quan8/go-ethereum/event"
	"github.com/quan8/go-ethereum/node"
	"github.com/quan8/go-ethereum/p2p"
	"github.com/quan8/go-ethereum/p2p/discv5"
	"github.com/quan8/go-ethereum/p2p/enr"
	"github.com/quan8/go-ethereum/params"
	"github.com/quan8/go-ethereum/rpc"

	"github.com/Fantom-foundation/go-lachesis/ethapi"
	"github.com/Fantom-foundation/go-lachesis/event_check"
	"github.com/Fantom-foundation/go-lachesis/evm_core"
	"github.com/Fantom-foundation/go-lachesis/gossip/gasprice"
	"github.com/Fantom-foundation/go-lachesis/gossip/occured_txs"
	"github.com/Fantom-foundation/go-lachesis/inter"
	"github.com/Fantom-foundation/go-lachesis/inter/idx"
	"github.com/Fantom-foundation/go-lachesis/logger"
)

const (
	txsRingBufferSize = 20000 // Maximum number of stored hashes of included but not confirmed txs
)

type ServiceFeed struct {
	newEpoch        notify.Feed
	newPack         notify.Feed
	newEmittedEvent notify.Feed
	newBlock        notify.Feed
	scope           notify.SubscriptionScope
}

func (f *ServiceFeed) SubscribeNewEpoch(ch chan<- idx.Epoch) notify.Subscription {
	return f.scope.Track(f.newEpoch.Subscribe(ch))
}

func (f *ServiceFeed) SubscribeNewPack(ch chan<- idx.Pack) notify.Subscription {
	return f.scope.Track(f.newPack.Subscribe(ch))
}

func (f *ServiceFeed) SubscribeNewBlock(ch chan<- evm_core.ChainHeadNotify) notify.Subscription {
	return f.scope.Track(f.newBlock.Subscribe(ch))
}

func (f *ServiceFeed) SubscribeNewEmitted(ch chan<- *inter.Event) notify.Subscription {
	return f.scope.Track(f.newEmittedEvent.Subscribe(ch))
}

// Service implements go-ethereum/node.Service interface.
type Service struct {
	config Config

	wg   sync.WaitGroup
	done chan struct{}

	// server
	Name  string
	Topic discv5.Topic

	serverPool *serverPool

	// application
	node        *node.ServiceContext
	store       *Store
	engine      Consensus
	engineMu    *sync.RWMutex
	emitter     *Emitter
	txpool      *evm_core.TxPool
	occurredTxs *occured_txs.Buffer

	feed ServiceFeed

	// application protocol
	pm *ProtocolManager

	EthAPI *EthAPIBackend

	logger.Instance
}

func NewService(ctx *node.ServiceContext, config Config, store *Store, engine Consensus) (*Service, error) {
	svc := &Service{
		config: config,

		done: make(chan struct{}),

		Name: fmt.Sprintf("Node-%d", rand.Int()),

		node:  ctx,
		store: store,

		engineMu:    new(sync.RWMutex),
		occurredTxs: occured_txs.New(txsRingBufferSize, types.NewEIP155Signer(params.AllEthashProtocolChanges.ChainID)),

		Instance: logger.MakeInstance(),
	}

	svc.engine = &HookedEngine{
		engine:       engine,
		processEvent: svc.processEvent,
	}
	svc.engine.Bootstrap(svc.onNewBlock)

	trustedNodes := []string{}
	svc.serverPool = newServerPool(store.table.Peers, svc.done, &svc.wg, trustedNodes)

	stateReader := svc.GetEvmStateReader()
	svc.txpool = evm_core.NewTxPool(config.TxPool, params.AllEthashProtocolChanges, stateReader)

	var err error
	svc.pm, err = NewProtocolManager(&config, &svc.feed, svc.txpool, svc.engineMu, store, svc.engine, svc.serverPool)

	svc.EthAPI = &EthAPIBackend{config.ExtRPCEnabled, svc, stateReader, nil, new(notify.TypeMux)}
	svc.EthAPI.gpo = gasprice.NewOracle(svc.EthAPI, svc.config.GPO)

	return svc, err
}

func (s *Service) processEvent(realEngine Consensus, e *inter.Event) error {
	// s.engineMu is locked here

	if s.store.HasEvent(e.Hash()) { // sanity check
		return event_check.ErrAlreadyConnectedEvent
	}

	oldEpoch := e.Epoch

	s.store.SetEvent(e)
	if realEngine != nil {
		err := realEngine.ProcessEvent(e)
		if err != nil { // TODO make it possible to write only on success
			s.store.DeleteEvent(e.Epoch, e.Hash())
			return err
		}
	}
	_ = s.occurredTxs.CollectNotConfirmedTxs(e.Transactions)

	// set validator's last event. we don't care about forks, because this index is used only for emitter
	s.store.SetLastEvent(e.Epoch, e.Creator, e.Hash())

	// track events with no descendants, i.e. heads
	for _, parent := range e.Parents {
		if s.store.IsHead(e.Epoch, parent) {
			s.store.DelHead(e.Epoch, parent)
		}
	}
	s.store.AddHead(e.Epoch, e.Hash())

	s.packs_onNewEvent(e, e.Epoch)
	s.emitter.OnNewEvent(e)

	newEpoch := oldEpoch
	if realEngine != nil {
		newEpoch = realEngine.GetEpoch()
	}

	if newEpoch != oldEpoch {
		s.packs_onNewEpoch(oldEpoch, newEpoch)
		s.store.delEpochStore(oldEpoch)
		s.store.getEpochStore(newEpoch)
		s.feed.newEpoch.Send(newEpoch)
		s.occurredTxs.Clear()
	}

	immediately := (newEpoch != oldEpoch)
	return s.store.Commit(e.Hash().Bytes(), immediately)
}

func (s *Service) makeEmitter() *Emitter {
	return NewEmitter(&s.config,
		EmitterWorld{
			Am:          s.AccountManager(),
			Engine:      s.engine,
			EngineMu:    s.engineMu,
			Store:       s.store,
			Txpool:      s.txpool,
			OccurredTxs: s.occurredTxs,
			OnEmitted: func(emitted *inter.Event) {
				// s.engineMu is locked here

				err := s.engine.ProcessEvent(emitted)
				if err != nil {
					s.Log.Crit("Self-event connection failed", "err", err.Error())
				}

				s.feed.newEmittedEvent.Send(emitted) // PM listens and will broadcast it
				if err != nil {
					s.Log.Crit("Failed to post self-event", "err", err.Error())
				}
			},
			IsSynced: func() bool {
				return atomic.LoadUint32(&s.pm.synced) != 0
			},
			PeersNum: func() int {
				return s.pm.peers.Len()
			},
		},
	)
}

// Protocols returns protocols the service can communicate on.
func (s *Service) Protocols() []p2p.Protocol {
	protos := make([]p2p.Protocol, len(ProtocolVersions))
	for i, vsn := range ProtocolVersions {
		protos[i] = s.pm.makeProtocol(vsn)
		protos[i].Attributes = []enr.Entry{s.currentEnr()}
	}
	return protos
}

// APIs returns api methods the service wants to expose on rpc channels.
func (s *Service) APIs() []rpc.API {
	apis := ethapi.GetAPIs(s.EthAPI)

	apis = append(apis, rpc.API{
		Namespace: "eth",
		Version:   "1.0",
		Service:   NewPublicEthereumAPI(s),
		Public:    true,
	})

	return apis
}

// Start method invoked when the node is ready to start the service.
func (s *Service) Start(srv *p2p.Server) error {

	var genesis common.Hash
	genesis = s.engine.GetGenesisHash()
	s.Topic = discv5.Topic("lachesis@" + genesis.Hex())

	if srv.DiscV5 != nil {
		go func(topic discv5.Topic) {
			s.Log.Info("Starting topic registration")
			defer s.Log.Info("Terminated topic registration")

			srv.DiscV5.RegisterTopic(topic, s.done)
		}(s.Topic)
	}

	s.pm.Start(srv.MaxPeers)

	s.serverPool.start(srv, s.Topic)

	s.emitter = s.makeEmitter()
	s.emitter.SetCoinbase(s.config.Emitter.Coinbase)
	s.emitter.StartEventEmission()

	return nil
}

// Stop method invoked when the node terminates the service.
func (s *Service) Stop() error {
	close(s.done)
	s.emitter.StopEventEmission()
	s.pm.Stop()
	s.wg.Wait()
	s.feed.scope.Close()

	// flush the state at exit, after all the routines stopped
	s.engineMu.Lock()
	defer s.engineMu.Unlock()
	return s.store.Commit(nil, true)
}

// AccountManager return node's account manager
func (s *Service) AccountManager() *accounts.Manager {
	return s.node.AccountManager
}
