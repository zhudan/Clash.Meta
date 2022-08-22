package tunnel

import (
	"context"
	"fmt"
	P "github.com/Dreamacro/clash/component/process"
	"net"
	"net/netip"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/Dreamacro/clash/adapter/inbound"
	"github.com/Dreamacro/clash/component/nat"
	"github.com/Dreamacro/clash/component/resolver"
	"github.com/Dreamacro/clash/component/sniffer"
	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/constant/provider"
	icontext "github.com/Dreamacro/clash/context"
	"github.com/Dreamacro/clash/log"
	"github.com/Dreamacro/clash/tunnel/statistic"
)

var (
	tcpQueue       = make(chan C.ConnContext, 200)
	udpQueue       = make(chan *inbound.PacketAdapter, 200)
	natTable       = nat.New()
	rules          []C.Rule
	proxies        = make(map[string]C.Proxy)
	providers      map[string]provider.ProxyProvider
	ruleProviders  map[string]provider.RuleProvider
	sniffingEnable bool
	configMux      sync.RWMutex

	// Outbound Rule
	mode = Rule

	// default timeout for UDP session
	udpTimeout = 60 * time.Second

	alwaysFindProcess = false
)

func SetSniffing(b bool) {
	if sniffer.Dispatcher.Enable() {
		configMux.Lock()
		sniffingEnable = b
		configMux.Unlock()
	}
}

func IsSniffing() bool {
	return sniffingEnable
}

func init() {
	go process()
}

// TCPIn return fan-in queue
func TCPIn() chan<- C.ConnContext {
	return tcpQueue
}

// UDPIn return fan-in udp queue
func UDPIn() chan<- *inbound.PacketAdapter {
	return udpQueue
}

// Rules return all rules
func Rules() []C.Rule {
	return rules
}

// UpdateRules handle update rules
func UpdateRules(newRules []C.Rule, rp map[string]provider.RuleProvider) {
	configMux.Lock()
	rules = newRules
	ruleProviders = rp
	configMux.Unlock()
}

// Proxies return all proxies
func Proxies() map[string]C.Proxy {
	return proxies
}

// Providers return all compatible providers
func Providers() map[string]provider.ProxyProvider {
	return providers
}

// RuleProviders return all loaded rule providers
func RuleProviders() map[string]provider.RuleProvider {
	return ruleProviders
}

// UpdateProxies handle update proxies
func UpdateProxies(newProxies map[string]C.Proxy, newProviders map[string]provider.ProxyProvider) {
	configMux.Lock()
	proxies = newProxies
	providers = newProviders
	configMux.Unlock()
}

func UpdateSniffer(dispatcher *sniffer.SnifferDispatcher) {
	configMux.Lock()
	sniffer.Dispatcher = *dispatcher
	sniffingEnable = true
	configMux.Unlock()
}

// Mode return current mode
func Mode() TunnelMode {
	return mode
}

// SetMode change the mode of tunnel
func SetMode(m TunnelMode) {
	mode = m
}

// SetAlwaysFindProcess set always find process info, may be increase many memory
func SetAlwaysFindProcess(findProcess bool) {
	alwaysFindProcess = findProcess
}

// processUDP starts a loop to handle udp packet
func processUDP() {
	queue := udpQueue
	for conn := range queue {
		handleUDPConn(conn)
	}
}

func process() {
	numUDPWorkers := 4
	if num := runtime.GOMAXPROCS(0); num > numUDPWorkers {
		numUDPWorkers = num
	}
	for i := 0; i < numUDPWorkers; i++ {
		go processUDP()
	}

	queue := tcpQueue
	for conn := range queue {
		go handleTCPConn(conn)
	}
}

func needLookupIP(metadata *C.Metadata) bool {
	return resolver.MappingEnabled() && metadata.Host == "" && metadata.DstIP.IsValid()
}

func preHandleMetadata(metadata *C.Metadata) error {
	// handle IP string on host
	if ip, err := netip.ParseAddr(metadata.Host); err == nil {
		metadata.DstIP = ip
		metadata.Host = ""
		if ip.Is4() {
			metadata.AddrType = C.AtypIPv4
		} else {
			metadata.AddrType = C.AtypIPv6
		}
	}

	// preprocess enhanced-mode metadata
	if needLookupIP(metadata) {
		host, exist := resolver.FindHostByIP(metadata.DstIP)
		if exist {
			metadata.Host = host
			metadata.AddrType = C.AtypDomainName
			metadata.DNSMode = C.DNSMapping
			if resolver.FakeIPEnabled() {
				metadata.DstIP = netip.Addr{}
				metadata.DNSMode = C.DNSFakeIP
			} else if node := resolver.DefaultHosts.Search(host); node != nil {
				// redir-host should lookup the hosts
				metadata.DstIP = node.Data
			}
		} else if resolver.IsFakeIP(metadata.DstIP) {
			return fmt.Errorf("fake DNS record %s missing", metadata.DstIP)
		}
	}

	return nil
}

func resolveMetadata(_ C.PlainContext, metadata *C.Metadata) (proxy C.Proxy, rule C.Rule, err error) {
	switch mode {
	case Direct:
		proxy = proxies["DIRECT"]
	case Global:
		proxy = proxies["GLOBAL"]
	// Rule
	default:
		proxy, rule, err = match(metadata)
	}
	return
}

func handleUDPConn(packet *inbound.PacketAdapter) {
	metadata := packet.Metadata()
	if !metadata.Valid() {
		log.Warnln("[Metadata] not valid: %#v", metadata)
		return
	}

	// make a fAddr if request ip is fakeip
	var fAddr netip.Addr
	if resolver.IsExistFakeIP(metadata.DstIP) {
		fAddr = metadata.DstIP
	}

	if err := preHandleMetadata(metadata); err != nil {
		log.Debugln("[Metadata PreHandle] error: %s", err)
		return
	}

	// local resolve UDP dns
	if !metadata.Resolved() {
		ip, err := resolver.ResolveIP(metadata.Host)
		if err != nil {
			return
		}
		metadata.DstIP = ip
	}

	key := packet.LocalAddr().String()

	handle := func() bool {
		pc := natTable.Get(key)
		if pc != nil {
			_ = handleUDPToRemote(packet, pc, metadata)
			return true
		}
		return false
	}

	if handle() {
		return
	}

	lockKey := key + "-lock"
	cond, loaded := natTable.GetOrCreateLock(lockKey)

	go func() {
		if loaded {
			cond.L.Lock()
			cond.Wait()
			handle()
			cond.L.Unlock()
			return
		}

		defer func() {
			natTable.Delete(lockKey)
			cond.Broadcast()
		}()

		pCtx := icontext.NewPacketConnContext(metadata)
		proxy, rule, err := resolveMetadata(pCtx, metadata)
		if err != nil {
			log.Warnln("[UDP] Parse metadata failed: %s", err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), C.DefaultUDPTimeout)
		defer cancel()
		rawPc, err := proxy.ListenPacketContext(ctx, metadata)
		if err != nil {
			if rule == nil {
				log.Warnln("[UDP] dial %s to %s error: %s", proxy.Name(), metadata.RemoteAddress(), err.Error())
			} else {
				log.Warnln("[UDP] dial %s (match %s/%s) to %s error: %s", proxy.Name(), rule.RuleType().String(), rule.Payload(), metadata.RemoteAddress(), err.Error())
			}
			return
		}
		pCtx.InjectPacketConn(rawPc)

		pc := statistic.NewUDPTracker(rawPc, statistic.DefaultManager, metadata, rule)

		switch true {
		case rule != nil:
			if rule.Payload() != "" {
				log.Infoln("[UDP] %s --> %s match %s using %s", metadata.SourceDetail(), metadata.RemoteAddress(), fmt.Sprintf("%s(%s)", rule.RuleType().String(), rule.Payload()), rawPc.Chains().String())
			} else {
				log.Infoln("[UDP] %s --> %s match %s using %s", metadata.SourceDetail(), metadata.RemoteAddress(), rule.Payload(), rawPc.Chains().String())
			}
		case mode == Global:
			log.Infoln("[UDP] %s --> %s using GLOBAL", metadata.SourceDetail(), metadata.RemoteAddress())
		case mode == Direct:
			log.Infoln("[UDP] %s --> %s using DIRECT", metadata.SourceDetail(), metadata.RemoteAddress())
		default:
			log.Infoln("[UDP] %s --> %s doesn't match any rule using DIRECT", metadata.SourceDetail(), metadata.RemoteAddress())
		}

		oAddr := metadata.DstIP
		go handleUDPToLocal(packet.UDPPacket, pc, key, oAddr, fAddr)

		natTable.Set(key, pc)
		handle()
	}()
}

func handleTCPConn(connCtx C.ConnContext) {
	defer func(conn net.Conn) {
		_ = conn.Close()
	}(connCtx.Conn())

	metadata := connCtx.Metadata()
	if !metadata.Valid() {
		log.Warnln("[Metadata] not valid: %#v", metadata)
		return
	}

	if err := preHandleMetadata(metadata); err != nil {
		log.Debugln("[Metadata PreHandle] error: %s", err)
		return
	}

	if sniffer.Dispatcher.Enable() && sniffingEnable {
		sniffer.Dispatcher.TCPSniff(connCtx.Conn(), metadata)
	}

	proxy, rule, err := resolveMetadata(connCtx, metadata)
	if err != nil {
		log.Warnln("[Metadata] parse failed: %s", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), C.DefaultTCPTimeout)
	defer cancel()
	remoteConn, err := proxy.DialContext(ctx, metadata)
	if err != nil {
		if rule == nil {
			log.Warnln("[TCP] dial %s to %s error: %s", proxy.Name(), metadata.RemoteAddress(), err.Error())
		} else {
			log.Warnln("[TCP] dial %s (match %s(%s)) to %s error: %s", proxy.Name(), rule.RuleType().String(), rule.Payload(), metadata.RemoteAddress(), err.Error())
		}
		return
	}

	remoteConn = statistic.NewTCPTracker(remoteConn, statistic.DefaultManager, metadata, rule)
	defer func(remoteConn C.Conn) {
		_ = remoteConn.Close()
	}(remoteConn)

	switch true {
	case rule != nil:
		if rule.Payload() != "" {
			log.Infoln("[TCP] %s --> %s match %s using %s", metadata.SourceDetail(), metadata.RemoteAddress(), fmt.Sprintf("%s(%s)", rule.RuleType().String(), rule.Payload()), remoteConn.Chains().String())
		} else {
			log.Infoln("[TCP] %s --> %s match %s using %s", metadata.SourceDetail(), metadata.RemoteAddress(), rule.RuleType().String(), remoteConn.Chains().String())
		}
	case mode == Global:
		log.Infoln("[TCP] %s --> %s using GLOBAL", metadata.SourceDetail(), metadata.RemoteAddress())
	case mode == Direct:
		log.Infoln("[TCP] %s --> %s using DIRECT", metadata.SourceDetail(), metadata.RemoteAddress())
	default:
		log.Infoln("[TCP] %s --> %s doesn't match any rule using DIRECT", metadata.SourceAddress(), metadata.RemoteAddress())
	}

	handleSocket(connCtx, remoteConn)
}

func shouldResolveIP(rule C.Rule, metadata *C.Metadata) bool {
	return rule.ShouldResolveIP() && metadata.Host != "" && !metadata.DstIP.IsValid()
}

func match(metadata *C.Metadata) (C.Proxy, C.Rule, error) {
	configMux.RLock()
	defer configMux.RUnlock()
	var (
		resolved     bool
		processFound bool
	)

	if node := resolver.DefaultHosts.Search(metadata.Host); node != nil {
		metadata.DstIP = node.Data
		resolved = true
	}

	for _, rule := range rules {
		if !resolved && shouldResolveIP(rule, metadata) {
			ip, err := resolver.ResolveIP(metadata.Host)
			if err != nil {
				log.Debugln("[DNS] resolve %s error: %s", metadata.Host, err.Error())
			} else {
				log.Debugln("[DNS] %s --> %s", metadata.Host, ip.String())
				metadata.DstIP = ip
			}
			resolved = true
		}

		if !processFound && (alwaysFindProcess || rule.ShouldFindProcess()) {
			srcPort, err := strconv.ParseUint(metadata.SrcPort, 10, 16)
			uid, path, err := P.FindProcessName(metadata.NetWork.String(), metadata.SrcIP, int(srcPort))
			if err != nil {
				log.Debugln("[Process] find process %s: %v", metadata.String(), err)
			} else {
				metadata.Process = filepath.Base(path)
				metadata.ProcessPath = path
				if uid != -1 {
					metadata.Uid = &uid
				}
				processFound = true
			}
		}

		if rule.Match(metadata) {
			adapter, ok := proxies[rule.Adapter()]
			if !ok {
				continue
			}

			if adapter.Type() == C.Pass || (adapter.Unwrap(metadata) != nil && adapter.Unwrap(metadata).Type() == C.Pass) {
				log.Debugln("%s match Pass rule", adapter.Name())
				continue
			}

			if metadata.NetWork == C.UDP && !adapter.SupportUDP() {
				log.Debugln("%s UDP is not supported", adapter.Name())
				continue
			}

			return adapter, rule, nil
		}
	}

	return proxies["DIRECT"], nil, nil
}
