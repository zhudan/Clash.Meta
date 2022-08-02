package shadowsocks

import (
	"github.com/Dreamacro/clash/adapter/inbound"
	"github.com/Dreamacro/clash/common/pool"
	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/log"
	"github.com/Dreamacro/clash/transport/socks5"
	"net"
)

type UDPListener struct {
	packetConn net.PacketConn
	addr       string
	closed     bool
}

// RawAddress implements C.Listener
func (l *UDPListener) RawAddress() string {
	return l.addr
}

// Address implements C.Listener
func (l *UDPListener) Address() string {
	return l.packetConn.LocalAddr().String()
}

// Close implements C.Listener
func (l *UDPListener) Close() error {
	l.closed = true
	return l.packetConn.Close()
}

// Listen on addr for encrypted packets and basically do UDP NAT.
func NewUDP(addr string, shadow func(net.PacketConn) net.PacketConn, in chan<- *inbound.PacketAdapter) (*UDPListener, error) {
	c, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Infoln("UDP remote listen error: %v", err)
		return nil, err
	}
	sl := &UDPListener{
		packetConn: c,
		addr:       addr,
	}
	c = shadow(c)
	go func() {
		for {
			buf := pool.Get(pool.UDPBufferSize)
			n, remoteAddr, err := c.ReadFrom(buf)
			if err != nil {
				pool.Put(buf)
				if sl.closed {
					break
				}
				continue
			}
			handleShadowsocksUDP(c, buf[:n], remoteAddr, in)
		}
	}()
	return sl, nil
}

func handleShadowsocksUDP(pc net.PacketConn, buf []byte, rAddr net.Addr, in chan<- *inbound.PacketAdapter) {
	tgtAddr := socks5.SplitAddr(buf)
	payload := buf[len(tgtAddr):len(buf)]
	packet := &packet{
		pc:      pc,
		rAddr:   rAddr,
		payload: payload,
		bufRef:  buf,
	}
	select {
	case in <- inbound.NewPacket(tgtAddr, packet, C.SHADOWSOCKS):
	default:
	}
}

type packet struct {
	pc      net.PacketConn
	rAddr   net.Addr
	payload []byte
	bufRef  []byte
}

func (c *packet) Data() []byte {
	return c.payload
}

// WriteBack write UDP packet with source(ip, port) = `addr`
func (c *packet) WriteBack(b []byte, addr net.Addr) (n int, err error) {
	packet, err := socks5.EncodeUDPPacket(socks5.ParseAddrToSocksAddr(addr), b)
	if err != nil {
		return
	}
	return c.pc.WriteTo(packet, c.rAddr)
}

// LocalAddr returns the source IP/Port of UDP Packet
func (c *packet) LocalAddr() net.Addr {
	return c.rAddr
}

func (c *packet) Drop() {
	pool.Put(c.bufRef)
}
