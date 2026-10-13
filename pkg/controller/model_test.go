// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Network interfaces are the multicast source, so bngblaster reports the
// tx-*-multicast keys there (access interfaces report rx-*-multicast).
func TestNetworkInterfacesResponse_multicast(t *testing.T) {
	payload := []byte(`{"status":"ok","code":200,"network-interfaces":[` +
		`{"name":"eth1","type":"Network","tx-packets-multicast":1000,"tx-pps-multicast":10}]}`)

	var cr NetworkInterfacesResponse
	require.NoError(t, json.Unmarshal(payload, &cr))
	require.Len(t, cr.Interfaces, 1)
	require.Equal(t, 1000, cr.Interfaces[0].TxPacketsMulticast)
	require.Equal(t, 10, cr.Interfaces[0].TxPPSMulticast)
}
