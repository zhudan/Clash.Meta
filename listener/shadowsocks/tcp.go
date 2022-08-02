package shadowsocks

import (
	"context"
	"github.com/Dreamacro/clash/adapter/inbound"
	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/log"
	"github.com/Dreamacro/clash/transport/socks5"
	"github.com/database64128/tfo-go"
	"net"
)

type Listener struct {
	listener net.Listener
	addr     string
	closed   bool
}

// RawAddress implements C.Listener
func (l *Listener) RawAddress() string {
	return l.addr
}

// Address implements C.Listener
func (l *Listener) Address() string {
	return l.listener.Addr().String()
}

// Close implements C.Listener
func (l *Listener) Close() error {
	l.closed = true
	return l.listener.Close()
}

// Listen on addr for incoming connections.
func New(addr string, shadow func(net.Conn) net.Conn, inboundTfo bool, in chan<- C.ConnContext) (*Listener, error) {
	lc := tfo.ListenConfig{
		DisableTFO: !inboundTfo,
	}
	l, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		log.Infoln("failed to listen on %s: %v", addr, err)
		return nil, err
	}
	sl := &Listener{
		listener: l,
		addr:     addr,
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				if sl.closed {
					break
				}
				continue
			}
			go handleShadowsocks(c, shadow, in)
		}
	}()
	return sl, nil
}

func handleShadowsocks(conn net.Conn, shadow func(net.Conn) net.Conn, in chan<- C.ConnContext) {
	conn.(*net.TCPConn).SetKeepAlive(true)
	sc := shadow(conn)
	tgt, err := socks5.ReadAddr(sc, make([]byte, socks5.MaxAddrLen))
	if err != nil {
		return
	}
	in <- inbound.NewSocket(tgt, sc, C.SHADOWSOCKS)
}
