package mixed

import (
	"context"
	"net"
	"time"

	"github.com/Dreamacro/clash/common/cache"
	N "github.com/Dreamacro/clash/common/net"
	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/listener/http"
	"github.com/Dreamacro/clash/listener/socks"
	"github.com/Dreamacro/clash/transport/socks4"
	"github.com/Dreamacro/clash/transport/socks5"
	"github.com/database64128/tfo-go"
)

type Listener struct {
	listener net.Listener
	addr     string
	cache    *cache.Cache[string, bool]
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

func New(addr string, inboundTfo bool, in chan<- C.ConnContext) (*Listener, error) {
	lc := tfo.ListenConfig{
		DisableTFO: !inboundTfo,
	}
	l, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, err
	}

	ml := &Listener{
		listener: l,
		addr:     addr,
		cache:    cache.New[string, bool](30 * time.Second),
	}
	go func() {
		for {
			c, err := ml.listener.Accept()
			if err != nil {
				if ml.closed {
					break
				}
				continue
			}
			go handleConn(c, in, ml.cache)
		}
	}()

	return ml, nil
}

func handleConn(conn net.Conn, in chan<- C.ConnContext, cache *cache.Cache[string, bool]) {
	conn.(*net.TCPConn).SetKeepAlive(true)

	bufConn := N.NewBufferedConn(conn)
	head, err := bufConn.Peek(1)
	if err != nil {
		return
	}

	switch head[0] {
	case socks4.Version:
		socks.HandleSocks4(bufConn, in)
	case socks5.Version:
		socks.HandleSocks5(bufConn, in)
	default:
		http.HandleConn(bufConn, in, cache)
	}
}
