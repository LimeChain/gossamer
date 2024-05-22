// Copyright 2023 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package wazero_runtime

import (
	"context"
	"github.com/ChainSafe/gossamer/lib/network"
	"os"
	"path/filepath"
	"testing"

	"github.com/ChainSafe/gossamer/internal/log"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/runtime/mocks"
	"github.com/ChainSafe/gossamer/lib/runtime/storage"
	"github.com/ChainSafe/gossamer/pkg/trie"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// DefaultTestLogLvl is the log level used for test runtime instances
var DefaultTestLogLvl = log.Trace

type TestInstanceOption func(*Config)

func TestWithLogLevel(lvl log.Level) TestInstanceOption {
	return func(c *Config) {
		c.LogLvl = lvl
	}
}

func TestWithTrie(tt *trie.Trie) TestInstanceOption {
	return func(c *Config) {
		c.Storage = storage.NewTrieState(tt)
	}
}

func TestWithVersion(version *runtime.Version) TestInstanceOption {
	return func(c *Config) {
		c.DefaultVersion = version
	}
}

func NewTestInstance(t *testing.T, targetRuntime string, opts ...TestInstanceOption) *Instance {
	t.Helper()

	ctrl := gomock.NewController(t)
	cfg := &Config{
		Storage:  storage.NewTrieState(trie.NewEmptyTrie()),
		Keystore: keystore.NewGlobalKeystore(),
		LogLvl:   DefaultTestLogLvl,
		NodeStorage: runtime.NodeStorage{
			LocalStorage:      runtime.NewInMemoryDB(t),
			PersistentStorage: runtime.NewInMemoryDB(t), // we're using a local storage here since this is a test runtime
			BaseDB:            runtime.NewInMemoryDB(t), // we're using a local storage here since this is a test runtime
		},
		Network:     new(runtime.TestRuntimeNetwork),
		Transaction: mocks.NewMockTransactionState(ctrl),
		Role:        network.NoNetworkRole,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	targetRuntime, err := runtime.GetRuntime(context.Background(), targetRuntime)
	require.NoError(t, err)

	r, err := NewInstanceFromFile(t, targetRuntime, *cfg)
	require.NoError(t, err)

	return r
}

// NewInstanceFromFile instantiates a runtime from a .wasm file
func NewInstanceFromFile(t *testing.T, targetRuntime string, cfg Config) (*Instance, error) {
	// Reads the WebAssembly module as bytes.
	// Retrieve WASM binary
	bytes, err := os.ReadFile(filepath.Clean(targetRuntime))
	require.NoError(t, err)

	r, err := NewInstance(bytes, cfg)
	require.NoError(t, err)

	return r, err
}

func NewBenchInstanceWithTrie(b *testing.B, targetRuntime string, tt *trie.Trie) *Instance {
	b.Helper()

	ctrl := gomock.NewController(b)

	cfg := setupBenchConfig(b, ctrl, tt, DefaultTestLogLvl, network.NoNetworkRole)
	targetRuntime, err := runtime.GetRuntime(context.Background(), targetRuntime)
	require.NoError(b, err)

	bytes, err := os.ReadFile(filepath.Clean(targetRuntime))
	require.NoError(b, err)

	r, err := NewInstance(bytes, cfg)
	require.NoError(b, err)

	return r
}

func setupBenchConfig(b *testing.B, ctrl *gomock.Controller, tt *trie.Trie, lvl log.Level, role network.NetworkRole) Config {
	b.Helper()

	s := storage.NewTrieState(tt)

	ns := runtime.NodeStorage{
		LocalStorage:      runtime.NewBenchInMemoryDB(b),
		PersistentStorage: runtime.NewBenchInMemoryDB(b),
		BaseDB:            runtime.NewBenchInMemoryDB(b),
	}

	return Config{
		Storage:     s,
		Keystore:    keystore.NewGlobalKeystore(),
		LogLvl:      lvl,
		NodeStorage: ns,
		Network:     new(runtime.TestRuntimeNetwork),
		Transaction: mocks.NewMockTransactionState(ctrl),
		Role:        role,
	}
}
