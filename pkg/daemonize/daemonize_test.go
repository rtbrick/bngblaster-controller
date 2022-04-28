package daemonize

import (
	"fmt"
	"testing"

	isf "github.com/matryer/is"
)

func TestDaemonize(t *testing.T) {
	is := isf.New(t)
	gotSig, err := Daemonize(func() error {
		return nil
	})
	is.NoErr(err)
	is.Equal(gotSig, NormalTerminationSignal{})

	gotSig, err = Daemonize(func() error {
		return fmt.Errorf("haha")
	})
	is.True(err != nil)
	is.Equal(gotSig, NormalTerminationSignal{})
}
