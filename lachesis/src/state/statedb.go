// Package state provides a caching layer atop the state trie.
package state

import (
	"errors"
	"fmt"
	"sort"

	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"

	"github.com/Fantom-foundation/go-lachesis/lachesis/src/crypto"
	"github.com/Fantom-foundation/go-lachesis/lachesis/src/hash"
	"github.com/Fantom-foundation/go-lachesis/lachesis/src/inter"
	"github.com/Fantom-foundation/go-lachesis/lachesis/src/inter/idx"
	"github.com/Fantom-foundation/go-lachesis/lachesis/src/trie"
)

type revision struct {
	id           int
	journalIndex int
}

var (
	// emptyState is the known hash of an empty state trie entry.
	emptyState = hash.Of(nil)
)

type proofList [][]byte

func (n *proofList) Put(key []byte, value []byte) error {
	*n = append(*n, value)
	return nil
}

// DB is used to store anything within the merkle trie.
// It takes care of caching and storing nested states.
// It's the general query interface to retrieve Accounts
type DB struct {
	db   Database
	trie Trie

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateObjects      map[hash.Peer]*stateObject
	stateObjectsDirty map[hash.Peer]struct{}

	// DB error.
	// State objects are used by the consensus core which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	thash, bhash hash.Hash
	txIndex      int

	preimages map[hash.Hash][]byte

	// Journal of state modifications. This is the backbone of
	// Snapshot and RevertToSnapshot.
	journal        *journal
	validRevisions []revision
	nextRevisionID int
}

// New creates a new state from a given trie.
func New(root hash.Hash, db Database) (*DB, error) {
	tr, err := db.OpenTrie(root)
	if err != nil {
		return nil, err
	}
	return &DB{
		db:                db,
		trie:              tr,
		stateObjects:      make(map[hash.Peer]*stateObject),
		stateObjectsDirty: make(map[hash.Peer]struct{}),
		preimages:         make(map[hash.Hash][]byte),
		journal:           newJournal(),
	}, nil
}

// setError remembers the first non-nil error it is called with.
func (s *DB) setError(err error) {
	if s.dbErr == nil {
		s.dbErr = err
	}
}

// Error returns the first non-nil database error
func (s *DB) Error() error {
	return s.dbErr
}

// Reset clears out all ephemeral state objects from the state db, but keeps
// the underlying state trie to avoid reloading data for the next operations.
func (s *DB) Reset(root hash.Hash) error {
	tr, err := s.db.OpenTrie(root)
	if err != nil {
		return err
	}
	s.trie = tr
	s.stateObjects = make(map[hash.Peer]*stateObject)
	s.stateObjectsDirty = make(map[hash.Peer]struct{})
	s.thash = hash.Hash{}
	s.bhash = hash.Hash{}
	s.txIndex = 0
	s.preimages = make(map[hash.Hash][]byte)
	s.clearJournal()
	return nil
}

// AddPreimage records a SHA3 preimage seen by the VM.
func (s *DB) AddPreimage(hash hash.Hash, preimage []byte) {
	if _, ok := s.preimages[hash]; !ok {
		s.journal.append(addPreimageChange{hash: hash})
		pi := make([]byte, len(preimage))
		copy(pi, preimage)
		s.preimages[hash] = pi
	}
}

// Preimages returns a list of SHA3 preimages that have been submitted.
func (s *DB) Preimages() map[hash.Hash][]byte {
	return s.preimages
}

// Exist reports whether the given account address exists in the state.
// Notably this also returns true for suicided accounts.
func (s *DB) Exist(addr hash.Peer) bool {
	return s.getStateObject(addr) != nil
}

// Empty returns whether the state object is either non-existent
// or empty according to the EIP161 specification (balance = nonce = code = 0).
func (s *DB) Empty(addr hash.Peer) bool {
	so := s.getStateObject(addr)
	return so == nil || so.empty()
}

// FreeBalance returns the free balance from the given address or 0 if object not found.
func (s *DB) FreeBalance(addr hash.Peer) inter.Stake {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return inter.Stake(stateObject.FreeBalance())
	}
	return inter.Stake(0)
}

// VoteBalance returns the vote balance from the given address or 0 if object not found.
func (s *DB) VoteBalance(addr hash.Peer) inter.Stake {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return inter.Stake(stateObject.VoteBalance())
	}
	return inter.Stake(0)
}

// GetState retrieves a value from the given account's storage trie.
func (s *DB) GetState(addr hash.Peer, h hash.Hash) hash.Hash {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetState(s.db, h)
	}
	return hash.Hash{}
}

// GetProof returns the MerkleProof for a given Account.
func (s *DB) GetProof(a hash.Peer) ([][]byte, error) {
	var proof proofList
	err := s.trie.Prove(crypto.Keccak256(a.Bytes()), 0, &proof)
	return [][]byte(proof), err
}

// GetStorageProof returns the StorageProof for given key.
func (s *DB) GetStorageProof(a hash.Peer, key hash.Hash) ([][]byte, error) {
	var proof proofList
	storageTrie := s.StorageTrie(a)
	if storageTrie == nil {
		return proof, errors.New("storage trie for requested address does not exist")
	}
	err := storageTrie.Prove(crypto.Keccak256(key.Bytes()), 0, &proof)
	return [][]byte(proof), err
}

// GetCommittedState retrieves a value from the given account's committed storage trie.
func (s *DB) GetCommittedState(addr hash.Peer, h hash.Hash) hash.Hash {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetCommittedState(s.db, h)
	}
	return hash.Hash{}
}

// Database retrieves the low level database supporting the lower level trie ops.
func (s *DB) Database() Database {
	return s.db
}

// StorageTrie returns the storage trie of an account.
// The return value is a copy and is nil for non-existent accounts.
func (s *DB) StorageTrie(addr hash.Peer) Trie {
	stateObject := s.getStateObject(addr)
	if stateObject == nil {
		return nil
	}
	cpy := stateObject.deepCopy(s)
	return cpy.updateTrie(s.db)
}

// HasSuicided checks if stateObject is suicided by address.
func (s *DB) HasSuicided(addr hash.Peer) bool {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.suicided
	}
	return false
}

/*
 * SETTERS
 */

// SetBalance sets stateObject's balance by address.
func (s *DB) SetBalance(addr hash.Peer, amount inter.Stake) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject == nil {
		panic("stateObject is nil")
	}
	stateObject.SetBalance(uint64(amount))
}

// Transfer moves amount.
func (s *DB) Transfer(from, to hash.Peer, amount inter.Stake) {
	f := s.GetOrNewStateObject(from)
	t := s.GetOrNewStateObject(to)

	f.SubBalance(uint64(amount))
	t.AddBalance(uint64(amount))
}

// Delegate writes delegation records.
func (s *DB) Delegate(from, to hash.Peer, amount inter.Stake, until idx.Block) {
	f := s.GetOrNewStateObject(from)
	t := s.GetOrNewStateObject(to)

	f.DelegateTo(to, int64(amount), uint64(until))
	t.DelegateTo(from, -1*int64(amount), uint64(until))
}

// ExpireDelegations erases data about expired delegations.
func (s *DB) ExpireDelegations(addr hash.Peer, now uint64) {
	stateObject := s.GetOrNewStateObject(addr)
	stateObject.ExpireDelegations(now)
}

// GetDelegations returns delegation records.
func (s *DB) GetDelegations(addr hash.Peer) [2]map[hash.Peer]uint64 {
	stateObject := s.GetOrNewStateObject(addr)
	return stateObject.GetDelegations()
}

// SetState sets stateObject's kv-state by address.
func (s *DB) SetState(addr hash.Peer, key, value hash.Hash) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject == nil {
		panic("stateObject is nil")
	}
	stateObject.SetState(s.db, key, value)
}

// Suicide marks the given account as suicided.
// This clears the account balance.
// The account's state object is still available until the state is committed,
// getStateObject will return a non-nil account after Suicide.
func (s *DB) Suicide(addr hash.Peer) bool {
	stateObject := s.getStateObject(addr)
	if stateObject == nil {
		return false
	}
	s.journal.append(suicideChange{
		account:  &addr,
		prev:     stateObject.suicided,
		prevData: stateObject.data,
	})
	stateObject.suicided = true
	stateObject.data = Account{}

	return true
}

//
// Setting, updating & deleting state object methods.
//

// updateStateObject writes the given object to the trie.
func (s *DB) updateStateObject(stateObject *stateObject) {
	addr := stateObject.Address()

	data, err := proto.Marshal(stateObject.Data())
	if err != nil {
		panic(fmt.Errorf("can't encode object at %s: %v", addr.String(), err))
	}

	s.setError(s.trie.TryUpdate(addr.Bytes(), data))
}

// deleteStateObject removes the given object from the state trie.
func (s *DB) deleteStateObject(stateObject *stateObject) {
	stateObject.deleted = true
	addr := stateObject.Address()
	s.setError(s.trie.TryDelete(addr.Bytes()))
}

// Retrieve a state object given by the address. Returns nil if not found.
func (s *DB) getStateObject(addr hash.Peer) (stateObject *stateObject) {
	// Prefer 'live' objects.
	if obj := s.stateObjects[addr]; obj != nil {
		if obj.deleted {
			return nil
		}
		return obj
	}

	// Load the object from the database.
	enc, err := s.trie.TryGet(addr.Bytes())
	if len(enc) == 0 {
		s.setError(err)
		return nil
	}
	var data Account
	if err := proto.Unmarshal(enc, &data); err != nil {
		log.Error("Failed to decode state object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newObject(s, addr, data)
	s.setStateObject(obj)
	return obj
}

func (s *DB) setStateObject(object *stateObject) {
	s.stateObjects[object.Address()] = object
}

// GetOrNewStateObject returns a state object or create a new state object if nil.
func (s *DB) GetOrNewStateObject(addr hash.Peer) *stateObject {
	stateObject := s.getStateObject(addr)
	if stateObject == nil || stateObject.deleted {
		stateObject, _ = s.createObject(addr)
	}
	return stateObject
}

// createObject creates a new state object. If there is an existing account with
// the given address, it is overwritten and returned as the second return value.
func (s *DB) createObject(addr hash.Peer) (newobj, prev *stateObject) {
	prev = s.getStateObject(addr)
	newobj = newObject(s, addr, Account{})
	if prev == nil {
		s.journal.append(createObjectChange{account: &addr})
	} else {
		s.journal.append(resetObjectChange{prev: prev})
	}
	s.setStateObject(newobj)
	return newobj, prev
}

// CreateAccount explicitly creates a state object. If a state object with the address
// already exists the balance is carried over to the new account.
//
// CreateAccount is called during the EVM CREATE operation. The situation might arise that
// a contract does the following:
//
//   1. sends funds to sha(account ++ (nonce + 1))
//   2. tx_create(sha(account ++ nonce)) (note that this gets the address of 1)
//
// Carrying over the balance ensures that Ether doesn't disappear.
func (s *DB) CreateAccount(addr hash.Peer) {
	_new, prev := s.createObject(addr)
	if prev != nil {
		_new.data = prev.data
	}
}

// ForEachStorage calls func for each key-value of node.
func (s *DB) ForEachStorage(addr hash.Peer, cb func(key, value hash.Hash) bool) {
	so := s.getStateObject(addr)
	if so == nil {
		return
	}
	it := trie.NewIterator(so.getTrie(s.db).NodeIterator(nil))
	for it.Next() {
		key := hash.FromBytes(s.trie.GetKey(it.Key))
		if value, dirty := so.dirtyStorage[key]; dirty {
			cb(key, value)
			continue
		}
		cb(key, hash.FromBytes(it.Value))
	}
}

// Copy creates a deep, independent copy of the state.
// Snapshots of the copied state cannot be applied to the copy.
func (s *DB) Copy() *DB {
	// Copy all the basic fields, initialize the memory ones
	state := &DB{
		db:                s.db,
		trie:              s.db.CopyTrie(s.trie),
		stateObjects:      make(map[hash.Peer]*stateObject, len(s.journal.dirties)),
		stateObjectsDirty: make(map[hash.Peer]struct{}, len(s.journal.dirties)),
		preimages:         make(map[hash.Hash][]byte),
		journal:           newJournal(),
	}
	// Copy the dirty states, logs, and preimages
	for addr := range s.journal.dirties {
		// As documented in the Finalise-method, there is a case where an object is
		// in the journal but not in the stateObjects: OOG after touch on ripeMD
		// prior to Byzantium. Thus, we need to check for nil
		if object, exist := s.stateObjects[addr]; exist {
			state.stateObjects[addr] = object.deepCopy(state)
			state.stateObjectsDirty[addr] = struct{}{}
		}
	}
	// Above, we don't copy the actual journal. This means that if the copy is copied, the
	// loop above will be a no-op, since the copy's journal is empty.
	// Thus, here we iterate over stateObjects, to enable copies of copies
	for addr := range s.stateObjectsDirty {
		if _, exist := state.stateObjects[addr]; !exist {
			state.stateObjects[addr] = s.stateObjects[addr].deepCopy(state)
			state.stateObjectsDirty[addr] = struct{}{}
		}
	}
	for hash_, preimage := range s.preimages {
		state.preimages[hash_] = preimage
	}
	return state
}

// Snapshot returns an identifier for the current revision of the state.
func (s *DB) Snapshot() int {
	id := s.nextRevisionID
	s.nextRevisionID++
	s.validRevisions = append(s.validRevisions, revision{id, s.journal.length()})
	return id
}

// RevertToSnapshot reverts all state changes made since the given revision.
func (s *DB) RevertToSnapshot(revID int) {
	// Find the snapshot in the stack of valid snapshots.
	idx := sort.Search(len(s.validRevisions), func(i int) bool {
		return s.validRevisions[i].id >= revID
	})
	if idx == len(s.validRevisions) || s.validRevisions[idx].id != revID {
		panic(fmt.Errorf("revision id %v cannot be reverted", revID))
	}
	snapshot := s.validRevisions[idx].journalIndex

	// Replay the journal to undo changes and remove invalidated snapshots
	s.journal.revert(s, snapshot)
	s.validRevisions = s.validRevisions[:idx]
}

// Finalise finalises the state by removing the self destructed objects
// and clears the journal as well as the refunds.
func (s *DB) Finalise(deleteEmptyObjects bool) {
	for addr := range s.journal.dirties {
		stateObject, exist := s.stateObjects[addr]
		if !exist {
			// Thus, we can safely ignore it here
			continue
		}

		if stateObject.suicided || (deleteEmptyObjects && stateObject.empty()) {
			s.deleteStateObject(stateObject)
		} else {
			stateObject.updateRoot(s.db)
			s.updateStateObject(stateObject)
		}
		s.stateObjectsDirty[addr] = struct{}{}
	}
	// Invalidate journal because reverting across transactions is not allowed.
	s.clearJournal()
}

// IntermediateRoot computes the current root hash of the state trie.
// It is called in between transactions to get the root hash that
// goes into transaction receipts.
func (s *DB) IntermediateRoot(deleteEmptyObjects bool) hash.Hash {
	s.Finalise(deleteEmptyObjects)
	return s.trie.Hash()
}

// Prepare sets the current transaction hash and index and block hash which is
// used when the EVM emits new state logs.
func (s *DB) Prepare(thash, bhash hash.Hash, ti int) {
	s.thash = thash
	s.bhash = bhash
	s.txIndex = ti
}

func (s *DB) clearJournal() {
	s.journal = newJournal()
	s.validRevisions = s.validRevisions[:0]
}

// Commit writes the state to the underlying in-memory trie database.
func (s *DB) Commit(deleteEmptyObjects bool) (root hash.Hash, err error) {
	defer s.clearJournal()

	for addr := range s.journal.dirties {
		s.stateObjectsDirty[addr] = struct{}{}
	}
	// Commit objects to the trie.
	for addr, stateObject := range s.stateObjects {
		_, isDirty := s.stateObjectsDirty[addr]
		switch {
		case stateObject.suicided || (isDirty && deleteEmptyObjects && stateObject.empty()):
			// If the object has been removed, don't bother syncing it
			// and just mark it for deletion in the trie.
			s.deleteStateObject(stateObject)
		case isDirty:
			// Write any storage changes in the state object to its storage trie.
			if err := stateObject.CommitTrie(s.db); err != nil {
				return hash.Hash{}, err
			}
			// Update the object in the main account trie.
			s.updateStateObject(stateObject)
		}
		delete(s.stateObjectsDirty, addr)
	}
	// Write trie changes.
	root, err = s.trie.Commit(func(leaf []byte, parent hash.Hash) error {
		var account Account
		if err := proto.Unmarshal(leaf, &account); err != nil {
			return nil
		}
		if account.Root() != emptyState {
			s.db.TrieDB().Reference(account.Root(), parent)
		}
		return nil
	})

	return root, err
}
