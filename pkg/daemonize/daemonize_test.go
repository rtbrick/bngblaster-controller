// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2024, RtBrick, Inc.
package daemonize

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDaemonize(t *testing.T) {
	gotSig, err := Daemonize(func() error {
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, gotSig, NormalTerminationSignal{})

	gotSig, err = Daemonize(func() error {
		return fmt.Errorf("haha")
	})
	require.Error(t, err)
	require.Equal(t, gotSig, NormalTerminationSignal{})
}
