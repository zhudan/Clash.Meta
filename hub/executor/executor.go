package executor

import (
	"fmt"
	"github.com/Dreamacro/clash/component/tls"
	"github.com/Dreamacro/clash/listener/inner"
	"net/netip"
	"os"
	"runtime"
	"sync"

	"github.com/Dreamacro/clash/adapter"
	"github.com/Dreamacro/clash/adapter/outboundgroup"
	"github.com/Dreamacro/clash/component/auth"
	"github.com/Dreamacro/clash/component/dialer"
	G "github.com/Dreamacro/clash/component/geodata"
	"github.com/Dreamacro/clash/component/iface"
	"github.com/Dreamacro/clash/component/profile"
	"github.com/Dreamacro/clash/component/profile/cachefile"
	"github.com/Dreamacro/clash/component/resolver"
	SNI "github.com/Dreamacro/clash/component/sniffer"
	"github.com/Dreamacro/clash/component/trie"
	"github.com/Dreamacro/clash/config"
	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/constant/provider"
	"github.com/Dreamacro/clash/dns"
	P "github.com/Dreamacro/clash/listener"
	authStore "github.com/Dreamacro/clash/listener/auth"
	"github.com/Dreamacro/clash/listener/tproxy"
	"github.com/Dreamacro/clash/log"
	"github.com/Dreamacro/clash/tunnel"
)

var mux sync.Mutex

func readConfig(path string) ([]byte, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("configuration file %s is empty", path)
	}

	return data, err
}

// Parse config with default config path
func Parse() (*config.Config, error) {
	return ParseWithPath(C.Path.Config())
}

// ParseWithPath parse config with custom config path
func ParseWithPath(path string) (*config.Config, error) {
	buf, err := readConfig(path)
	if err != nil {
		return nil, err
	}

	return ParseWithBytes(buf)
}

// ParseWithBytes config with buffer
func ParseWithBytes(buf []byte) (*config.Config, error) {
	return config.Parse(buf)
}

// ApplyConfig dispatch configure to all parts
func ApplyConfig(cfg *config.Config, force bool) {
	mux.Lock()
	defer mux.Unlock()
	preUpdateExperimental(cfg)
	updateUsers(cfg.Users)
	updateProxies(cfg.Proxies, cfg.Providers)
	updateRules(cfg.Rules, cfg.RuleProviders)
	updateSniffer(cfg.Sniffer)
	updateHosts(cfg.Hosts)
	initInnerTcp()
	updateDNS(cfg.DNS, cfg.General.IPv6)
	loadProxyProvider(cfg.Providers)
	updateProfile(cfg)
	loadRuleProvider(cfg.RuleProviders)
	updateGeneral(cfg.General, force)
	updateIPTables(cfg)
	updateTun(cfg.Tun)
	updateExperimental(cfg)

	// DON'T Delete
	// ClashX will use this line to determine if the 'Meta' has finished booting
	log.Infoln("Apply all configs finished.")

	log.SetLevel(cfg.General.LogLevel)
}

func initInnerTcp() {
	inner.New(tunnel.TCPIn())
}

func GetGeneral() *config.General {
	ports := P.GetPorts()
	var authenticator []string
	if auth := authStore.Authenticator(); auth != nil {
		authenticator = auth.Users()
	}

	general := &config.General{
		Inbound: config.Inbound{
			Port:           ports.Port,
			SocksPort:      ports.SocksPort,
			RedirPort:      ports.RedirPort,
			TProxyPort:     ports.TProxyPort,
			MixedPort:      ports.MixedPort,
			Authentication: authenticator,
			AllowLan:       P.AllowLan(),
			BindAddress:    P.BindAddress(),
		},
		Mode:          tunnel.Mode(),
		LogLevel:      log.Level(),
		IPv6:          !resolver.DisableIPv6,
		GeodataLoader: G.LoaderName(),
		Tun:           P.GetTunConf(),
		Interface:     dialer.DefaultInterface.Load(),
		Sniffing:      tunnel.IsSniffing(),
		TCPConcurrent: dialer.GetDial(),
	}

	return general
}

func updateExperimental(c *config.Config) {
	runtime.GC()
}

func preUpdateExperimental(c *config.Config) {
	for _, fingerprint := range c.Experimental.Fingerprints {
		if err := tls.AddCertFingerprint(fingerprint); err != nil {
			log.Warnln("fingerprint[%s] is err, %s", fingerprint, err.Error())
		}
	}
}

func updateDNS(c *config.DNS, generalIPv6 bool) {
	if !c.Enable {
		resolver.DisableIPv6 = !generalIPv6
		resolver.DefaultResolver = nil
		resolver.DefaultHostMapper = nil
		resolver.DefaultLocalServer = nil
		dns.ReCreateServer("", nil, nil)
		return
	} else {
		resolver.DisableIPv6 = !c.IPv6
	}

	cfg := dns.Config{
		Main:         c.NameServer,
		Fallback:     c.Fallback,
		IPv6:         c.IPv6,
		EnhancedMode: c.EnhancedMode,
		Pool:         c.FakeIPRange,
		Hosts:        c.Hosts,
		FallbackFilter: dns.FallbackFilter{
			GeoIP:     c.FallbackFilter.GeoIP,
			GeoIPCode: c.FallbackFilter.GeoIPCode,
			IPCIDR:    c.FallbackFilter.IPCIDR,
			Domain:    c.FallbackFilter.Domain,
			GeoSite:   c.FallbackFilter.GeoSite,
		},
		Default:     c.DefaultNameserver,
		Policy:      c.NameServerPolicy,
		ProxyServer: c.ProxyServerNameserver,
		PreferH3:    c.PreferH3,
	}

	r := dns.NewResolver(cfg)
	pr := dns.NewProxyServerHostResolver(r)
	m := dns.NewEnhancer(cfg)

	// reuse cache of old host mapper
	if old := resolver.DefaultHostMapper; old != nil {
		m.PatchFrom(old.(*dns.ResolverEnhancer))
	}

	resolver.DefaultResolver = r
	resolver.DefaultHostMapper = m
	resolver.DefaultLocalServer = dns.NewLocalServer(r, m)

	if pr.HasProxyServer() {
		resolver.ProxyServerHostResolver = pr
	}

	dns.ReCreateServer(c.Listen, r, m)
}

func updateHosts(tree *trie.DomainTrie[netip.Addr]) {
	resolver.DefaultHosts = tree
}

func updateProxies(proxies map[string]C.Proxy, providers map[string]provider.ProxyProvider) {
	tunnel.UpdateProxies(proxies, providers)
}

func updateRules(rules []C.Rule, ruleProviders map[string]provider.RuleProvider) {
	tunnel.UpdateRules(rules, ruleProviders)
}

func loadProvider(pv provider.Provider) {
	if pv.VehicleType() == provider.Compatible {
		log.Infoln("Start initial compatible provider %s", pv.Name())
	} else {
		log.Infoln("Start initial provider %s", (pv).Name())
	}

	if err := pv.Initial(); err != nil {
		switch pv.Type() {
		case provider.Proxy:
			{
				log.Warnln("initial proxy provider %s error: %v", (pv).Name(), err)
			}
		case provider.Rule:
			{
				log.Warnln("initial rule provider %s error: %v", (pv).Name(), err)
			}

		}
	}
}

func loadRuleProvider(ruleProviders map[string]provider.RuleProvider) {
	wg := sync.WaitGroup{}
	ch := make(chan struct{}, concurrentCount)
	for _, ruleProvider := range ruleProviders {
		ruleProvider := ruleProvider
		wg.Add(1)
		ch <- struct{}{}
		go func() {
			defer func() { <-ch; wg.Done() }()
			loadProvider(ruleProvider)

		}()
	}

	wg.Wait()
}

func loadProxyProvider(proxyProviders map[string]provider.ProxyProvider) {
	// limit concurrent size
	wg := sync.WaitGroup{}
	ch := make(chan struct{}, concurrentCount)
	for _, proxyProvider := range proxyProviders {
		proxyProvider := proxyProvider
		wg.Add(1)
		ch <- struct{}{}
		go func() {
			defer func() { <-ch; wg.Done() }()
			loadProvider(proxyProvider)
		}()
	}

	wg.Wait()
}

func updateTun(tun *config.Tun) {
	P.ReCreateTun(tun, tunnel.TCPIn(), tunnel.UDPIn())
}

func updateSniffer(sniffer *config.Sniffer) {
	if sniffer.Enable {
		dispatcher, err := SNI.NewSnifferDispatcher(sniffer.Sniffers, sniffer.ForceDomain, sniffer.SkipDomain, sniffer.Ports)
		if err != nil {
			log.Warnln("initial sniffer failed, err:%v", err)
		}

		tunnel.UpdateSniffer(dispatcher)
		log.Infoln("Sniffer is loaded and working")
	} else {
		dispatcher, err := SNI.NewCloseSnifferDispatcher()
		if err != nil {
			log.Warnln("initial sniffer failed, err:%v", err)
		}

		tunnel.UpdateSniffer(dispatcher)
		log.Infoln("Sniffer is closed")
	}
}

func updateGeneral(general *config.General, force bool) {
	tunnel.SetMode(general.Mode)
	tunnel.SetAlwaysFindProcess(general.EnableProcess)
	dialer.DisableIPv6 = !general.IPv6
	if !dialer.DisableIPv6 {
		log.Infoln("Use IPv6")
	} else {
		resolver.DisableIPv6 = true
	}

	if general.TCPConcurrent {
		dialer.SetDial(general.TCPConcurrent)
		log.Infoln("Use tcp concurrent")
	}

	adapter.UnifiedDelay.Store(general.UnifiedDelay)
	dialer.DefaultInterface.Store(general.Interface)

	if dialer.DefaultInterface.Load() != "" {
		log.Infoln("Use interface name: %s", general.Interface)
	}

	dialer.DefaultRoutingMark.Store(int32(general.RoutingMark))
	if general.RoutingMark > 0 {
		log.Infoln("Use routing mark: %#x", general.RoutingMark)
	}

	iface.FlushCache()

	if !force {
		return
	}

	geodataLoader := general.GeodataLoader
	G.SetLoader(geodataLoader)

	allowLan := general.AllowLan
	P.SetAllowLan(allowLan)

	bindAddress := general.BindAddress
	P.SetBindAddress(bindAddress)

	P.SetInboundTfo(general.InboundTfo)

	tcpIn := tunnel.TCPIn()
	udpIn := tunnel.UDPIn()

	P.ReCreateHTTP(general.Port, tcpIn)
	P.ReCreateSocks(general.SocksPort, tcpIn, udpIn)
	P.ReCreateRedir(general.RedirPort, tcpIn, udpIn)
	P.ReCreateTProxy(general.TProxyPort, tcpIn, udpIn)
	P.ReCreateMixed(general.MixedPort, tcpIn, udpIn)
}

func updateUsers(users []auth.AuthUser) {
	authenticator := auth.NewAuthenticator(users)
	authStore.SetAuthenticator(authenticator)
	if authenticator != nil {
		log.Infoln("Authentication of local server updated")
	}
}

func updateProfile(cfg *config.Config) {
	profileCfg := cfg.Profile

	profile.StoreSelected.Store(profileCfg.StoreSelected)
	if profileCfg.StoreSelected {
		patchSelectGroup(cfg.Proxies)
	}
}

func patchSelectGroup(proxies map[string]C.Proxy) {
	mapping := cachefile.Cache().SelectedMap()
	if mapping == nil {
		return
	}

	for name, proxy := range proxies {
		outbound, ok := proxy.(*adapter.Proxy)
		if !ok {
			continue
		}

		selector, ok := outbound.ProxyAdapter.(outboundgroup.SelectAble)
		if !ok {
			continue
		}

		selected, exist := mapping[name]
		if !exist {
			continue
		}

		selector.Set(selected)
	}
}

func updateIPTables(cfg *config.Config) {
	tproxy.CleanupTProxyIPTables()

	iptables := cfg.IPTables
	if runtime.GOOS != "linux" || !iptables.Enable {
		return
	}

	var err error
	defer func() {
		if err != nil {
			log.Errorln("[IPTABLES] setting iptables failed: %s", err.Error())
			os.Exit(2)
		}
	}()

	if cfg.Tun.Enable {
		err = fmt.Errorf("when tun is enabled, iptables cannot be set automatically")
		return
	}

	var (
		inboundInterface = "lo"
		bypass           = iptables.Bypass
		tProxyPort       = cfg.General.TProxyPort
		dnsCfg           = cfg.DNS
	)

	if tProxyPort == 0 {
		err = fmt.Errorf("tproxy-port must be greater than zero")
		return
	}

	if !dnsCfg.Enable {
		err = fmt.Errorf("DNS server must be enable")
		return
	}

	dnsPort, err := netip.ParseAddrPort(dnsCfg.Listen)
	if err != nil {
		err = fmt.Errorf("DNS server must be correct")
		return
	}

	if iptables.InboundInterface != "" {
		inboundInterface = iptables.InboundInterface
	}

	if dialer.DefaultRoutingMark.Load() == 0 {
		dialer.DefaultRoutingMark.Store(2158)
	}

	err = tproxy.SetTProxyIPTables(inboundInterface, bypass, uint16(tProxyPort), dnsPort.Port())
	if err != nil {
		return
	}

	log.Infoln("[IPTABLES] Setting iptables completed")
}

func Shutdown() {
	P.Cleanup(false)
	tproxy.CleanupTProxyIPTables()
	resolver.StoreFakePoolState()

	log.Warnln("Clash shutting down")
}
