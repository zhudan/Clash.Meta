//go:build !linux

package dialer

import (
	"github.com/database64128/tfo-go"
	"net"
	"net/netip"
	"sync"

	"github.com/Dreamacro/clash/log"
)

var printMarkWarnOnce sync.Once

func printMarkWarn() {
	printMarkWarnOnce.Do(func() {
		log.Warnln("Routing mark on socket is not supported on current platform")
	})
}

func bindMarkToDialer(mark int, dialer *tfo.Dialer, _ string, _ netip.Addr) {
	printMarkWarn()
}

func bindMarkToListenConfig(mark int, lc *net.ListenConfig, _, _ string) {
	printMarkWarn()
}
