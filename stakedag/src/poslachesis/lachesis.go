package lachesis

import (
	"go.etcd.io/bbolt"
	"google.golang.org/grpc"

	"github.com/Fantom-foundation/go-lachesis/src/crypto"
	"github.com/Fantom-foundation/go-lachesis/src/kvdb"
	"github.com/Fantom-foundation/go-lachesis/src/logger"
	"github.com/Fantom-foundation/go-lachesis/src/network"
	"github.com/Fantom-foundation/go-lachesis/stakedag/src/posnode"
	"github.com/Fantom-foundation/go-lachesis/stakedag/src/posposet"
)

// Lachesis is a lachesis node implementation.
type Lachesis struct {
	host           string
	conf           *Config
	node           *posnode.Node
	nodeStore      *posnode.Store
	consensus      *posposet.Poset
	consensusStore *posposet.Store

	service

	logger.Instance
}

// New makes lachesis node.
// It does not start any process.
func New(db *bbolt.DB, host string, key *crypto.PrivateKey, conf *Config, opts ...grpc.DialOption) *Lachesis {
	return makeLachesis(db, host, key, conf, nil, opts...)
}

func makeLachesis(db *bbolt.DB, host string, key *crypto.PrivateKey, conf *Config, listen network.ListenFunc, opts ...grpc.DialOption) *Lachesis {
	ndb, cdb := makeStorages(db)

	if conf == nil {
		conf = DefaultConfig()
	}

	c := posposet.New(cdb, ndb)
	n := posnode.New(host, key, ndb, c, &conf.Node, listen, opts...)

	return &Lachesis{
		host:           host,
		conf:           conf,
		node:           n,
		nodeStore:      ndb,
		consensus:      c,
		consensusStore: cdb,

		service: service{listen, nil},

		Instance: logger.MakeInstance(),
	}
}

func (l *Lachesis) init() {
	genesis := l.conf.Net.Genesis
	err := l.consensusStore.ApplyGenesis(genesis)
	if err != nil {
		l.Fatal(err)
	}
}

// Start inits and starts whole lachesis node.
func (l *Lachesis) Start() {
	l.init()

	l.consensus.Start()
	l.node.Start()
	l.serviceStart()
}

// Stop stops whole lachesis node.
func (l *Lachesis) Stop() {
	l.serviceStop()
	l.node.Stop()
	l.consensus.Stop()
}

// AddPeers suggests hosts for network discovery.
func (l *Lachesis) AddPeers(hosts ...string) {
	l.node.AddBuiltInPeers(hosts...)
}

/*
 * Utils:
 */

func makeStorages(db *bbolt.DB) (*posnode.Store, *posposet.Store) {
	var (
		p      kvdb.Database
		n      kvdb.Database
		cached bool
	)
	if db == nil {
		p = kvdb.NewMemDatabase()
		n = kvdb.NewMemDatabase()
		cached = false
	} else {
		db := kvdb.NewBoltDatabase(db)
		p = db.NewTable([]byte("p_"))
		n = db.NewTable([]byte("n_"))
		cached = true
	}

	return posnode.NewStore(n),
		posposet.NewStore(p, cached)
}
