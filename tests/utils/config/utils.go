// Copyright 2023 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package config

import (
	cfg "github.com/ChainSafe/gossamer/config"
	"github.com/ChainSafe/gossamer/lib/network"
)

// ParseNetworkRole converts a common.NetworkRole to a string representation.
func ParseNetworkRole(r network.NetworkRole) string {
	switch r {
	case network.NoNetworkRole:
		return cfg.NoNetworkRole.String()
	case network.FullNodeRole:
		return cfg.FullNode.String()
	case network.LightClientRole:
		return cfg.LightNode.String()
	case network.AuthorityRole:
		return cfg.AuthorityNode.String()
	default:
		return "Unknown"
	}
}
