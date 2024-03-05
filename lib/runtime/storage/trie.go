// Copyright 2021 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package storage

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/pkg/trie"
	"golang.org/x/exp/maps"
)

type TrackedStorageKey struct {
	// Key         string
	Reads       uint32
	Writes      uint32
	Whitelisted bool
}

// Create a default `TrackedStorageKey`
func NewTrackedStorageKey(key string) TrackedStorageKey {
	return TrackedStorageKey{
		// Key:         key,
		Reads:       0,
		Writes:      0,
		Whitelisted: false,
	}
}

// Check if this key has been "read", i.e. it exists in the memory overlay.
//
// Can be true if the key has been read, has been written to, or has been
// whitelisted.
func (tsk *TrackedStorageKey) HasBeenRead() bool {
	return tsk.Whitelisted || tsk.Reads > 0 || tsk.HasBeenWritten()
}

// Check if this key has been "written", i.e. a new value will be committed to the database.
//
// Can be true if the key has been written to, or has been whitelisted.
func (tsk *TrackedStorageKey) HasBeenWritten() bool {
	return tsk.Whitelisted || tsk.Writes > 0
}

// Add a storage read to this key.
func (tsk *TrackedStorageKey) AddRead() {
	tsk.Reads += 1
}

// Add a storage write to this key.
func (tsk *TrackedStorageKey) AddWrite() {
	tsk.Writes += 1
}

// Whitelist this key.
func (tsk *TrackedStorageKey) Whitelist() {
	tsk.Whitelisted = true
}

type KeyTracker struct {
	lock           sync.Mutex
	enableTracking bool

	// // Key tracker for keys in the main trie.
	// // We track the total number of reads and writes to these keys,
	// // not de-duplicated for repeats.
	// mainKeys LinkedHashMap[TrackedStorageKey]

	// // Key tracker for keys in a child trie.
	// // Child trie are identified by their storage key (i.e. `ChildInfo::storage_key()`)
	// // We track the total number of reads and writes to these keys,
	// // not de-duplicated for repeats.
	// childKeys NestedLinkedHashMap[TrackedStorageKey]

	keys map[string]TrackedStorageKey
}

func NewKeyTracker() KeyTracker {
	return KeyTracker{
		lock:           sync.Mutex{},
		enableTracking: false,
		keys:           map[string]TrackedStorageKey{},
	}
}

func (tr *KeyTracker) WipeTracker() {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.enableTracking = false
	for _, k := range tr.keys {
		k.Reads = 0
		k.Writes = 0
	}
}

func (tr *KeyTracker) Enable() {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.enableTracking = true
}

func (tr *KeyTracker) Disable() {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.enableTracking = false
}

func (tr *KeyTracker) Reads() uint32 {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	sum := uint32(0)
	for _, tk := range tr.keys {
		sum += tk.Reads
	}
	return sum
}

func (tr *KeyTracker) Writes() uint32 {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	sum := uint32(0)
	for _, tk := range tr.keys {
		sum += tk.Writes
	}
	return sum
}

func (tr *KeyTracker) addReadKey(key string) {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	if !tr.enableTracking {
		return
	}

	sk := tr.keys[key]
	if !sk.HasBeenRead() {
		sk.AddRead()
		tr.keys[key] = sk
	}
}

func (tr *KeyTracker) addWriteKey(key string) {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	if !tr.enableTracking {
		return
	}

	sk := tr.keys[key]
	if !sk.HasBeenWritten() {
		sk.AddWrite()
		tr.keys[key] = sk
	}
}

// TrieState is a wrapper around a transient trie that is used during the course of executing some runtime call.
// If the execution of the call is successful, the trie will be saved in the StorageState.
type TrieState struct {
	mtx           sync.RWMutex
	transactions  *list.List
	duplicateTrie *trie.Trie
	keyTracker    KeyTracker
}

func (s *TrieState) DbWhitelistKey(key string) {
	tsk := NewTrackedStorageKey(key)
	tsk.Whitelist()
	s.keyTracker.keys[key] = tsk
}

func (s *TrieState) DbResetTracker() {
	s.keyTracker.WipeTracker()
}

func (s *TrieState) DbStartTracker() {
	s.keyTracker.Enable()
}

func (s *TrieState) DbStopTracker() {
	s.keyTracker.Disable()
}

func (s *TrieState) DbReadCount() uint32 {
	return s.keyTracker.Reads()
}

func (s *TrieState) DbWriteCount() uint32 {
	return s.keyTracker.Writes()
}

func (s *TrieState) DbWipe() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	// TODO: there is no caching layer
	// changes (commited + reverted) from the overlay/cache are commited once per block
}

func (s *TrieState) DbCommit() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	// TODO: there is no caching layer
	// changes (commited + reverted) from the overlay/cache are commited once per block
}

func (s *TrieState) DbStoreSnapshot() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.duplicateTrie = s.getCurrentTrie().Snapshot()
	s.updateCurrentTrie(s.getCurrentTrie().Snapshot())
}

func (s *TrieState) DbRestoreSnapshot() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.updateCurrentTrie(s.duplicateTrie)
}

func NewTrieState(state *trie.Trie) *TrieState {
	transactions := list.New()
	transactions.PushBack(state)
	return &TrieState{
		transactions: transactions,
		keyTracker:   NewKeyTracker(),
	}
}

func (t *TrieState) getCurrentTrie() *trie.Trie {
	return t.transactions.Back().Value.(*trie.Trie)
}

func (t *TrieState) updateCurrentTrie(new *trie.Trie) {
	t.transactions.Back().Value = new
}

// StartTransaction begins a new nested storage transaction
// which will either be committed or rolled back at a later time.
func (t *TrieState) StartTransaction() {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	t.transactions.PushBack(t.getCurrentTrie().Snapshot())
}

// Rollback rolls back all storage changes made since StartTransaction was called.
func (t *TrieState) RollbackTransaction() {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	if t.transactions.Len() <= 1 {
		panic("no transactions to rollback")
	}

	t.transactions.Remove(t.transactions.Back())
}

// Commit commits all storage changes made since StartTransaction was called.
func (t *TrieState) CommitTransaction() {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	if t.transactions.Len() <= 1 {
		panic("no transactions to commit")
	}

	t.transactions.Back().Prev().Value = t.transactions.Remove(t.transactions.Back())
}

func (t *TrieState) SetVersion(v trie.TrieLayout) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.getCurrentTrie().SetVersion(v)
}

// Trie returns the TrieState's underlying trie
func (t *TrieState) Trie() *trie.Trie {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.getCurrentTrie()
}

// Snapshot creates a new "version" of the trie. The trie before Snapshot is called
// can no longer be modified, all further changes are on a new "version" of the trie.
// It returns the new version of the trie.
func (t *TrieState) Snapshot() *trie.Trie {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.getCurrentTrie().Snapshot()
}

// Put puts a key-value pair in the trie
func (t *TrieState) Put(key, value []byte) (err error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.keyTracker.addWriteKey(string(key))
	return t.getCurrentTrie().Put(key, value)
}

// Get gets a value from the trie
func (t *TrieState) Get(key []byte) []byte {
	t.mtx.RLock()
	defer t.mtx.RUnlock()
	t.keyTracker.addReadKey(string(key))
	return t.getCurrentTrie().Get(key)
}

// MustRoot returns the trie's root hash. It panics if it fails to compute the root.
func (t *TrieState) MustRoot() common.Hash {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.getCurrentTrie().MustHash()
}

// Root returns the trie's root hash
func (t *TrieState) Root() (common.Hash, error) {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.getCurrentTrie().Hash()
}

// Has returns whether or not a key exists
func (t *TrieState) Has(key []byte) bool {
	return t.Get(key) != nil
}

// Delete deletes a key from the trie
func (t *TrieState) Delete(key []byte) (err error) {
	val := t.getCurrentTrie().Get(key)
	if val == nil {
		return nil
	}

	t.mtx.Lock()
	defer t.mtx.Unlock()

	err = t.getCurrentTrie().Delete(key)
	if err != nil {
		return fmt.Errorf("deleting from trie: %w", err)
	}

	return nil
}

// NextKey returns the next key in the trie in lexicographical order. If it does not exist, it returns nil.
func (t *TrieState) NextKey(key []byte) []byte {
	t.mtx.RLock()
	defer t.mtx.RUnlock()
	return t.getCurrentTrie().NextKey(key)
}

// ClearPrefix deletes all key-value pairs from the trie where the key starts with the given prefix
func (t *TrieState) ClearPrefix(prefix []byte) (err error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.getCurrentTrie().ClearPrefix(prefix)
}

// ClearPrefixLimit deletes key-value pairs from the trie where the key starts with the given prefix till limit reached
func (t *TrieState) ClearPrefixLimit(prefix []byte, limit uint32) (
	deleted uint32, allDeleted bool, err error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	return t.getCurrentTrie().ClearPrefixLimit(prefix, limit)
}

// TrieEntries returns every key-value pair in the trie
func (t *TrieState) TrieEntries() map[string][]byte {
	t.mtx.RLock()
	defer t.mtx.RUnlock()
	return t.getCurrentTrie().Entries()
}

// SetChild sets the child trie at the given key
func (t *TrieState) SetChild(keyToChild []byte, child *trie.Trie) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.getCurrentTrie().SetChild(keyToChild, child)
}

// SetChildStorage sets a key-value pair in a child trie
func (t *TrieState) SetChildStorage(keyToChild, key, value []byte) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	return t.getCurrentTrie().PutIntoChild(keyToChild, key, value)
}

// GetChild returns the child trie at the given key
func (t *TrieState) GetChild(keyToChild []byte) (*trie.Trie, error) {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.getCurrentTrie().GetChild(keyToChild)
}

// GetChildStorage returns a value from a child trie
func (t *TrieState) GetChildStorage(keyToChild, key []byte) ([]byte, error) {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.getCurrentTrie().GetFromChild(keyToChild, key)
}

// DeleteChild deletes a child trie from the main trie
func (t *TrieState) DeleteChild(key []byte) (err error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	return t.getCurrentTrie().DeleteChild(key)
}

// DeleteChildLimit deletes up to limit of database entries by lexicographic order.
func (t *TrieState) DeleteChildLimit(key []byte, limit *[]byte) (
	deleted uint32, allDeleted bool, err error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	trieSnapshot := t.getCurrentTrie().Snapshot()

	tr, err := trieSnapshot.GetChild(key)
	if err != nil {
		return 0, false, err
	}

	childTrieEntries := tr.Entries()
	qtyEntries := uint32(len(childTrieEntries))
	if limit == nil {
		err = trieSnapshot.DeleteChild(key)
		if err != nil {
			return 0, false, fmt.Errorf("deleting child trie: %w", err)
		}

		t.updateCurrentTrie(trieSnapshot)
		return qtyEntries, true, nil
	}
	limitUint := binary.LittleEndian.Uint32(*limit)

	keys := maps.Keys(childTrieEntries)
	sort.Strings(keys)
	for _, k := range keys {
		// TODO have a transactional/atomic way to delete multiple keys in trie.
		// If one deletion fails, the child trie and its parent trie are then in
		// a bad intermediary state. Take also care of the caching of deleted Merkle
		// values within the tries, which is used for online pruning.
		// See https://github.com/ChainSafe/gossamer/issues/3032
		err = tr.Delete([]byte(k))
		if err != nil {
			return deleted, allDeleted, fmt.Errorf("deleting from child trie located at key 0x%x: %w", key, err)
		}

		deleted++
		if deleted == limitUint {
			break
		}
	}
	t.updateCurrentTrie(trieSnapshot)
	allDeleted = deleted == qtyEntries
	return deleted, allDeleted, nil
}

// ClearChildStorage removes the child storage entry from the trie
func (t *TrieState) ClearChildStorage(keyToChild, key []byte) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.getCurrentTrie().ClearFromChild(keyToChild, key)
}

// ClearPrefixInChild clears all the keys from the child trie that have the given prefix
func (t *TrieState) ClearPrefixInChild(keyToChild, prefix []byte) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	child, err := t.getCurrentTrie().GetChild(keyToChild)
	if err != nil {
		return err
	}
	if child == nil {
		return nil
	}

	err = child.ClearPrefix(prefix)
	if err != nil {
		return fmt.Errorf("clearing prefix in child trie located at key 0x%x: %w", keyToChild, err)
	}

	return nil
}

func (t *TrieState) ClearPrefixInChildWithLimit(keyToChild, prefix []byte, limit uint32) (uint32, bool, error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	child, err := t.getCurrentTrie().GetChild(keyToChild)
	if err != nil || child == nil {
		return 0, false, err
	}

	return child.ClearPrefixLimit(prefix, limit)
}

// GetChildNextKey returns the next lexicographical larger key from child storage. If it does not exist, it returns nil.
func (t *TrieState) GetChildNextKey(keyToChild, key []byte) ([]byte, error) {
	t.mtx.RLock()
	defer t.mtx.RUnlock()
	child, err := t.getCurrentTrie().GetChild(keyToChild)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, nil
	}
	return child.NextKey(key), nil
}

// GetKeysWithPrefixFromChild ...
func (t *TrieState) GetKeysWithPrefixFromChild(keyToChild, prefix []byte) ([][]byte, error) {
	child, err := t.GetChild(keyToChild)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, nil
	}
	return child.GetKeysWithPrefix(prefix), nil
}

// LoadCode returns the runtime code (located at :code)
func (t *TrieState) LoadCode() []byte {
	return t.Get(common.CodeKey)
}

// LoadCodeHash returns the hash of the runtime code (located at :code)
func (t *TrieState) LoadCodeHash() (common.Hash, error) {
	code := t.LoadCode()
	return common.Blake2bHash(code)
}

// GetChangedNodeHashes returns the two sets of hashes for all nodes
// inserted and deleted in the state trie since the last block produced (trie snapshot).
func (t *TrieState) GetChangedNodeHashes() (inserted, deleted map[common.Hash]struct{}, err error) {
	t.mtx.RLock()
	defer t.mtx.RUnlock()
	return t.getCurrentTrie().GetChangedNodeHashes()
}
