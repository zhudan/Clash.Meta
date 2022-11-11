package proxy

import (
	"fmt"
	"github.com/Dreamacro/clash/listener/sing_tun"
	"golang.org/x/exp/slices"
	"net"
	"sort"
	"strconv"
	"sync"

	"github.com/Dreamacro/clash/adapter/inbound"
	"github.com/Dreamacro/clash/component/ebpf"
	"github.com/Dreamacro/clash/config"
	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/listener/autoredir"
	"github.com/Dreamacro/clash/listener/http"
	"github.com/Dreamacro/clash/listener/inner"
	"github.com/Dreamacro/clash/listener/mixed"
	"github.com/Dreamacro/clash/listener/redir"
	"github.com/Dreamacro/clash/listener/socks"
	"github.com/Dreamacro/clash/listener/tproxy"
	"github.com/Dreamacro/clash/log"
	"github.com/Dreamacro/clash/transport/shadowsocks/core"
	"github.com/Dreamacro/clash/listener/shadowsocks"
)

var (
	allowLan    = false
	bindAddress = "*"
	inboundTfo  = false

	socksListener     *socks.Listener
	socksUDPListener  *socks.UDPListener
	httpListener      *http.Listener
	redirListener     *redir.Listener
	redirUDPListener  *tproxy.UDPListener
	tproxyListener    *tproxy.Listener
	tproxyUDPListener *tproxy.UDPListener
	mixedListener     *mixed.Listener
	mixedUDPLister    *socks.UDPListener
	tunLister         *sing_tun.Listener
	autoRedirListener *autoredir.Listener
	autoRedirProgram  *ebpf.TcEBpfProgram
	tcProgram         *ebpf.TcEBpfProgram
	shadowsocksListener    *shadowsocks.Listener
	shadowsocksUDPListener *shadowsocks.UDPListener

	// lock for recreate function
	socksMux       sync.Mutex
	httpMux        sync.Mutex
	redirMux       sync.Mutex
	tproxyMux      sync.Mutex
	mixedMux       sync.Mutex
	tunMux         sync.Mutex
	autoRedirMux   sync.Mutex
	tcMux          sync.Mutex
	shadowsocksMux sync.Mutex
	LastTunConf    config.Tun
)

type Ports struct {
	Port       int `json:"port"`
	SocksPort  int `json:"socks-port"`
	RedirPort  int `json:"redir-port"`
	TProxyPort int `json:"tproxy-port"`
	MixedPort  int `json:"mixed-port"`
}

func GetTunConf() config.Tun {
	if tunLister == nil {
		return config.Tun{
			Enable: false,
		}
	}
	return tunLister.Config()
}

func AllowLan() bool {
	return allowLan
}

func BindAddress() string {
	return bindAddress
}

func SetAllowLan(al bool) {
	allowLan = al
}

func SetBindAddress(host string) {
	bindAddress = host
}

func SetInboundTfo(itfo bool) {
	inboundTfo = itfo
}

func NewInner(tcpIn chan<- C.ConnContext) {
	inner.New(tcpIn)
}

func ReCreateHTTP(port int, tcpIn chan<- C.ConnContext) {
	httpMux.Lock()
	defer httpMux.Unlock()

	var err error
	defer func() {
		if err != nil {
			log.Errorln("Start HTTP server error: %s", err.Error())
		}
	}()

	addr := genAddr(bindAddress, port, allowLan)

	if httpListener != nil {
		if httpListener.RawAddress() == addr {
			return
		}
		httpListener.Close()
		httpListener = nil
	}

	if portIsZero(addr) {
		return
	}

	httpListener, err = http.New(addr, inboundTfo, tcpIn)
	if err != nil {
		log.Errorln("Start HTTP server error: %s", err.Error())
		return
	}

	log.Infoln("HTTP proxy listening at: %s", httpListener.Address())
}

func ReCreateSocks(port int, tcpIn chan<- C.ConnContext, udpIn chan<- *inbound.PacketAdapter) {
	socksMux.Lock()
	defer socksMux.Unlock()

	var err error
	defer func() {
		if err != nil {
			log.Errorln("Start SOCKS server error: %s", err.Error())
		}
	}()

	addr := genAddr(bindAddress, port, allowLan)

	shouldTCPIgnore := false
	shouldUDPIgnore := false

	if socksListener != nil {
		if socksListener.RawAddress() != addr {
			socksListener.Close()
			socksListener = nil
		} else {
			shouldTCPIgnore = true
		}
	}

	if socksUDPListener != nil {
		if socksUDPListener.RawAddress() != addr {
			socksUDPListener.Close()
			socksUDPListener = nil
		} else {
			shouldUDPIgnore = true
		}
	}

	if shouldTCPIgnore && shouldUDPIgnore {
		return
	}

	if portIsZero(addr) {
		return
	}

	tcpListener, err := socks.New(addr, inboundTfo, tcpIn)
	if err != nil {
		return
	}

	udpListener, err := socks.NewUDP(addr, udpIn)
	if err != nil {
		tcpListener.Close()
		return
	}

	socksListener = tcpListener
	socksUDPListener = udpListener

	log.Infoln("SOCKS proxy listening at: %s", socksListener.Address())
}

func ReCreateRedir(port int, tcpIn chan<- C.ConnContext, udpIn chan<- *inbound.PacketAdapter) {
	redirMux.Lock()
	defer redirMux.Unlock()

	var err error
	defer func() {
		if err != nil {
			log.Errorln("Start Redir server error: %s", err.Error())
		}
	}()

	addr := genAddr(bindAddress, port, allowLan)

	if redirListener != nil {
		if redirListener.RawAddress() == addr {
			return
		}
		redirListener.Close()
		redirListener = nil
	}

	if redirUDPListener != nil {
		if redirUDPListener.RawAddress() == addr {
			return
		}
		redirUDPListener.Close()
		redirUDPListener = nil
	}

	if portIsZero(addr) {
		return
	}

	redirListener, err = redir.New(addr, tcpIn)
	if err != nil {
		return
	}

	redirUDPListener, err = tproxy.NewUDP(addr, udpIn)
	if err != nil {
		log.Warnln("Failed to start Redir UDP Listener: %s", err)
	}

	log.Infoln("Redirect proxy listening at: %s", redirListener.Address())
}

func ReCreateTProxy(port int, tcpIn chan<- C.ConnContext, udpIn chan<- *inbound.PacketAdapter) {
	tproxyMux.Lock()
	defer tproxyMux.Unlock()

	var err error
	defer func() {
		if err != nil {
			log.Errorln("Start TProxy server error: %s", err.Error())
		}
	}()

	addr := genAddr(bindAddress, port, allowLan)

	if tproxyListener != nil {
		if tproxyListener.RawAddress() == addr {
			return
		}
		tproxyListener.Close()
		tproxyListener = nil
	}

	if tproxyUDPListener != nil {
		if tproxyUDPListener.RawAddress() == addr {
			return
		}
		tproxyUDPListener.Close()
		tproxyUDPListener = nil
	}

	if portIsZero(addr) {
		return
	}

	tproxyListener, err = tproxy.New(addr, tcpIn)
	if err != nil {
		return
	}

	tproxyUDPListener, err = tproxy.NewUDP(addr, udpIn)
	if err != nil {
		log.Warnln("Failed to start TProxy UDP Listener: %s", err)
	}

	log.Infoln("TProxy server listening at: %s", tproxyListener.Address())
}

func ReCreateMixed(port int, tcpIn chan<- C.ConnContext, udpIn chan<- *inbound.PacketAdapter) {
	mixedMux.Lock()
	defer mixedMux.Unlock()

	var err error
	defer func() {
		if err != nil {
			log.Errorln("Start Mixed(http+socks) server error: %s", err.Error())
		}
	}()

	addr := genAddr(bindAddress, port, allowLan)

	shouldTCPIgnore := false
	shouldUDPIgnore := false

	if mixedListener != nil {
		if mixedListener.RawAddress() != addr {
			mixedListener.Close()
			mixedListener = nil
		} else {
			shouldTCPIgnore = true
		}
	}
	if mixedUDPLister != nil {
		if mixedUDPLister.RawAddress() != addr {
			mixedUDPLister.Close()
			mixedUDPLister = nil
		} else {
			shouldUDPIgnore = true
		}
	}

	if shouldTCPIgnore && shouldUDPIgnore {
		return
	}

	if portIsZero(addr) {
		return
	}

	mixedListener, err = mixed.New(addr, inboundTfo, tcpIn)
	if err != nil {
		return
	}

	mixedUDPLister, err = socks.NewUDP(addr, udpIn)
	if err != nil {
		mixedListener.Close()
		return
	}

	log.Infoln("Mixed(http+socks) proxy listening at: %s", mixedListener.Address())
}

func ReCreateTun(tunConf config.Tun, tcpIn chan<- C.ConnContext, udpIn chan<- *inbound.PacketAdapter) {
	tunMux.Lock()
	defer func() {
		LastTunConf = tunConf
		tunMux.Unlock()
	}()

	var err error
	defer func() {
		if err != nil {
			log.Errorln("Start TUN listening error: %s", err.Error())
			Cleanup(false)
		}
	}()

	if !hasTunConfigChange(&tunConf) {
		if tunLister != nil {
			tunLister.FlushDefaultInterface()
		}
		return
	}

	Cleanup(true)

	if !tunConf.Enable {
		return
	}

	tunLister, err = sing_tun.New(tunConf, tcpIn, udpIn)
}

func ReCreateRedirToTun(ifaceNames []string) {
	tcMux.Lock()
	defer tcMux.Unlock()

	nicArr := ifaceNames
	slices.Sort(nicArr)
	nicArr = slices.Compact(nicArr)

	if tcProgram != nil {
		tcProgram.Close()
		tcProgram = nil
	}

	if len(nicArr) == 0 {
		return
	}

	tunConf := GetTunConf()

	if !tunConf.Enable {
		return
	}

	program, err := ebpf.NewTcEBpfProgram(nicArr, tunConf.Device)
	if err != nil {
		log.Errorln("Attached tc ebpf program error: %v", err)
		return
	}
	tcProgram = program

	log.Infoln("Attached tc ebpf program to interfaces %v", tcProgram.RawNICs())
}

func ReCreateAutoRedir(ifaceNames []string, tcpIn chan<- C.ConnContext, _ chan<- *inbound.PacketAdapter) {
	autoRedirMux.Lock()
	defer autoRedirMux.Unlock()

	var err error
	defer func() {
		if err != nil {
			if autoRedirListener != nil {
				_ = autoRedirListener.Close()
				autoRedirListener = nil
			}
			if autoRedirProgram != nil {
				autoRedirProgram.Close()
				autoRedirProgram = nil
			}
			log.Errorln("Start auto redirect server error: %s", err.Error())
		}
	}()

	nicArr := ifaceNames
	slices.Sort(nicArr)
	nicArr = slices.Compact(nicArr)

	if autoRedirListener != nil && autoRedirProgram != nil {
		_ = autoRedirListener.Close()
		autoRedirProgram.Close()
		autoRedirListener = nil
		autoRedirProgram = nil
	}

	if len(nicArr) == 0 {
		return
	}

	defaultRouteInterfaceName, err := ebpf.GetAutoDetectInterface()
	if err != nil {
		return
	}

	addr := genAddr("*", C.TcpAutoRedirPort, true)

	autoRedirListener, err = autoredir.New(addr, tcpIn)
	if err != nil {
		return
	}

	autoRedirProgram, err = ebpf.NewRedirEBpfProgram(nicArr, autoRedirListener.TCPAddr().Port(), defaultRouteInterfaceName)
	if err != nil {
		return
	}

	autoRedirListener.SetLookupFunc(autoRedirProgram.Lookup)

	log.Infoln("Auto redirect proxy listening at: %s, attached tc ebpf program to interfaces %v", autoRedirListener.Address(), autoRedirProgram.RawNICs())
}

func ReCreateShadowsocks(port int, cipher string, password string, tcpIn chan<- C.ConnContext, udpIn chan<- *inbound.PacketAdapter) {
	shadowsocksMux.Lock()
	defer shadowsocksMux.Unlock()

	var err error
	defer func() {
		if err != nil {
			log.Errorln("Start Shadowsocks server error: %s", err.Error())
		}
	}()

	addr := genAddr(bindAddress, port, allowLan)

	shouldTCPIgnore := false
	shouldUDPIgnore := false

	if shadowsocksListener != nil {
		if shadowsocksListener.RawAddress() != addr {
			shadowsocksListener.Close()
			shadowsocksListener = nil
		} else {
			shouldTCPIgnore = true
		}
	}

	if shadowsocksUDPListener != nil {
		if shadowsocksUDPListener.RawAddress() != addr {
			shadowsocksUDPListener.Close()
			shadowsocksUDPListener = nil
		} else {
			shouldUDPIgnore = true
		}
	}

	if shouldTCPIgnore && shouldUDPIgnore {
		return
	}

	if portIsZero(addr) {
		return
	}

	ciph, err := core.PickCipher(cipher, nil, password)
	tcpListener, err := shadowsocks.New(addr, ciph.StreamConn, inboundTfo, tcpIn)
	if err != nil {
		return
	}

	udpListener, err := shadowsocks.NewUDP(addr, ciph.PacketConn, udpIn)
	if err != nil {
		tcpListener.Close()
		return
	}
	shadowsocksListener = tcpListener
	shadowsocksUDPListener = udpListener
	log.Infoln("Shadowsocks proxy listening at: %s", shadowsocksListener.Address())
}

// GetPorts return the ports of proxy servers
func GetPorts() *Ports {
	ports := &Ports{}

	if httpListener != nil {
		_, portStr, _ := net.SplitHostPort(httpListener.Address())
		port, _ := strconv.Atoi(portStr)
		ports.Port = port
	}

	if socksListener != nil {
		_, portStr, _ := net.SplitHostPort(socksListener.Address())
		port, _ := strconv.Atoi(portStr)
		ports.SocksPort = port
	}

	if redirListener != nil {
		_, portStr, _ := net.SplitHostPort(redirListener.Address())
		port, _ := strconv.Atoi(portStr)
		ports.RedirPort = port
	}

	if tproxyListener != nil {
		_, portStr, _ := net.SplitHostPort(tproxyListener.Address())
		port, _ := strconv.Atoi(portStr)
		ports.TProxyPort = port
	}

	if mixedListener != nil {
		_, portStr, _ := net.SplitHostPort(mixedListener.Address())
		port, _ := strconv.Atoi(portStr)
		ports.MixedPort = port
	}

	return ports
}

func portIsZero(addr string) bool {
	_, port, err := net.SplitHostPort(addr)
	if port == "0" || port == "" || err != nil {
		return true
	}
	return false
}

func genAddr(host string, port int, allowLan bool) string {
	if allowLan {
		if host == "*" {
			return fmt.Sprintf(":%d", port)
		}
		return fmt.Sprintf("%s:%d", host, port)
	}

	return fmt.Sprintf("127.0.0.1:%d", port)
}

func hasTunConfigChange(tunConf *config.Tun) bool {
	if LastTunConf.Enable != tunConf.Enable ||
		LastTunConf.Device != tunConf.Device ||
		LastTunConf.Stack != tunConf.Stack ||
		LastTunConf.AutoRoute != tunConf.AutoRoute ||
		LastTunConf.AutoDetectInterface != tunConf.AutoDetectInterface ||
		LastTunConf.MTU != tunConf.MTU ||
		LastTunConf.StrictRoute != tunConf.StrictRoute ||
		LastTunConf.EndpointIndependentNat != tunConf.EndpointIndependentNat ||
		LastTunConf.UDPTimeout != tunConf.UDPTimeout {
		return true
	}

	if len(LastTunConf.DNSHijack) != len(tunConf.DNSHijack) {
		return true
	}

	sort.Slice(tunConf.DNSHijack, func(i, j int) bool {
		return tunConf.DNSHijack[i].Addr().Less(tunConf.DNSHijack[j].Addr())
	})

	sort.Slice(tunConf.Inet4Address, func(i, j int) bool {
		return tunConf.Inet4Address[i].Build().String() < tunConf.Inet4Address[j].Build().String()
	})

	sort.Slice(tunConf.Inet6Address, func(i, j int) bool {
		return tunConf.Inet6Address[i].Build().String() < tunConf.Inet6Address[j].Build().String()
	})

	sort.Slice(tunConf.Inet4RouteAddress, func(i, j int) bool {
		return tunConf.Inet4RouteAddress[i].Build().String() < tunConf.Inet4RouteAddress[j].Build().String()
	})

	sort.Slice(tunConf.Inet6RouteAddress, func(i, j int) bool {
		return tunConf.Inet6RouteAddress[i].Build().String() < tunConf.Inet6RouteAddress[j].Build().String()
	})

	sort.Slice(tunConf.IncludeUID, func(i, j int) bool {
		return tunConf.IncludeUID[i] < tunConf.IncludeUID[j]
	})

	sort.Slice(tunConf.IncludeUIDRange, func(i, j int) bool {
		return tunConf.IncludeUIDRange[i] < tunConf.IncludeUIDRange[j]
	})

	sort.Slice(tunConf.ExcludeUID, func(i, j int) bool {
		return tunConf.ExcludeUID[i] < tunConf.ExcludeUID[j]
	})

	sort.Slice(tunConf.ExcludeUIDRange, func(i, j int) bool {
		return tunConf.ExcludeUIDRange[i] < tunConf.ExcludeUIDRange[j]
	})

	sort.Slice(tunConf.IncludeAndroidUser, func(i, j int) bool {
		return tunConf.IncludeAndroidUser[i] < tunConf.IncludeAndroidUser[j]
	})

	sort.Slice(tunConf.IncludePackage, func(i, j int) bool {
		return tunConf.IncludePackage[i] < tunConf.IncludePackage[j]
	})

	sort.Slice(tunConf.ExcludePackage, func(i, j int) bool {
		return tunConf.ExcludePackage[i] < tunConf.ExcludePackage[j]
	})

	if !slices.Equal(tunConf.DNSHijack, LastTunConf.DNSHijack) ||
		!slices.Equal(tunConf.Inet4Address, LastTunConf.Inet4Address) ||
		!slices.Equal(tunConf.Inet6Address, LastTunConf.Inet6Address) ||
		!slices.Equal(tunConf.Inet4RouteAddress, LastTunConf.Inet4RouteAddress) ||
		!slices.Equal(tunConf.Inet6RouteAddress, LastTunConf.Inet6RouteAddress) ||
		!slices.Equal(tunConf.IncludeUID, LastTunConf.IncludeUID) ||
		!slices.Equal(tunConf.IncludeUIDRange, LastTunConf.IncludeUIDRange) ||
		!slices.Equal(tunConf.ExcludeUID, LastTunConf.ExcludeUID) ||
		!slices.Equal(tunConf.ExcludeUIDRange, LastTunConf.ExcludeUIDRange) ||
		!slices.Equal(tunConf.IncludeAndroidUser, LastTunConf.IncludeAndroidUser) ||
		!slices.Equal(tunConf.IncludePackage, LastTunConf.IncludePackage) ||
		!slices.Equal(tunConf.ExcludePackage, LastTunConf.ExcludePackage) {
		return true
	}

	return false
}

func Cleanup(wait bool) {
	if tunLister != nil {
		tunLister.Close()
		tunLister = nil
	}
	LastTunConf = config.Tun{}
}
