package resolver

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/netip"
	"time"

	"github.com/Dreamacro/clash/common/nnip"
	"github.com/Dreamacro/clash/component/trie"
)

var (
	// DefaultResolver aim to resolve ip
	DefaultResolver Resolver

	// ProxyServerHostResolver resolve ip to proxies server host
	ProxyServerHostResolver Resolver

	// DisableIPv6 means don't resolve ipv6 host
	// default value is true
	DisableIPv6 = true

	// DefaultHosts aim to resolve hosts
	DefaultHosts = trie.New[netip.Addr]()

	// DefaultDNSTimeout defined the default dns request timeout
	DefaultDNSTimeout = time.Second * 5
)

var (
	ErrIPNotFound   = errors.New("couldn't find ip")
	ErrIPVersion    = errors.New("ip version error")
	ErrIPv6Disabled = errors.New("ipv6 disabled")
)

type Resolver interface {
	ResolveIP(host string) (ip netip.Addr, err error)
	ResolveIPv4(host string) (ip netip.Addr, err error)
	ResolveIPv6(host string) (ip netip.Addr, err error)
	ResolveAllIP(host string) (ip []netip.Addr, err error)
	ResolveAllIPv4(host string) (ips []netip.Addr, err error)
	ResolveAllIPv6(host string) (ips []netip.Addr, err error)
}

// ResolveIPv4 with a host, return ipv4
func ResolveIPv4(host string) (netip.Addr, error) {
	return ResolveIPv4WithResolver(host, DefaultResolver)
}

func ResolveIPv4WithResolver(host string, r Resolver) (netip.Addr, error) {
	if ips, err := ResolveAllIPv4WithResolver(host, r); err == nil {
		return ips[rand.Intn(len(ips))], nil
	} else {
		return netip.Addr{}, err
	}
}

// ResolveIPv6 with a host, return ipv6
func ResolveIPv6(host string) (netip.Addr, error) {
	return ResolveIPv6WithResolver(host, DefaultResolver)
}

func ResolveIPv6WithResolver(host string, r Resolver) (netip.Addr, error) {
	if ips, err := ResolveAllIPv6WithResolver(host, r); err == nil {
		return ips[rand.Intn(len(ips))], nil
	} else {
		return netip.Addr{}, err
	}
}

// ResolveIPWithResolver same as ResolveIP, but with a resolver
func ResolveIPWithResolver(host string, r Resolver) (netip.Addr, error) {
	if ip, err := ResolveIPv4WithResolver(host, r); err == nil {
		return ip, nil
	} else {
		return ResolveIPv6WithResolver(host, r)
	}
}

// ResolveIP with a host, return ip
func ResolveIP(host string) (netip.Addr, error) {
	return ResolveIPWithResolver(host, DefaultResolver)
}

// ResolveIPv4ProxyServerHost proxies server host only
func ResolveIPv4ProxyServerHost(host string) (netip.Addr, error) {
	if ProxyServerHostResolver != nil {
		if ip, err := ResolveIPv4WithResolver(host, ProxyServerHostResolver); err != nil {
			return ResolveIPv4(host)
		} else {
			return ip, nil
		}
	}
	return ResolveIPv4(host)
}

// ResolveIPv6ProxyServerHost proxies server host only
func ResolveIPv6ProxyServerHost(host string) (netip.Addr, error) {
	if ProxyServerHostResolver != nil {
		if ip, err := ResolveIPv6WithResolver(host, ProxyServerHostResolver); err != nil {
			return ResolveIPv6(host)
		} else {
			return ip, nil
		}
	}
	return ResolveIPv6(host)
}

// ResolveProxyServerHost proxies server host only
func ResolveProxyServerHost(host string) (netip.Addr, error) {
	if ProxyServerHostResolver != nil {
		if ip, err := ResolveIPWithResolver(host, ProxyServerHostResolver); err != nil {
			return ResolveIP(host)
		} else {
			return ip, err
		}
	}
	return ResolveIP(host)
}

func ResolveAllIPv6WithResolver(host string, r Resolver) ([]netip.Addr, error) {
	if DisableIPv6 {
		return []netip.Addr{}, ErrIPv6Disabled
	}

	if node := DefaultHosts.Search(host); node != nil {
		if ip := node.Data(); ip.Is6() {
			return []netip.Addr{ip}, nil
		}
	}

	ip, err := netip.ParseAddr(host)
	if err == nil {
		if ip.Is6() {
			return []netip.Addr{ip}, nil
		}
		return []netip.Addr{}, ErrIPVersion
	}

	if r != nil {
		return r.ResolveAllIPv6(host)
	}

	if DefaultResolver == nil {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultDNSTimeout)
		defer cancel()
		ipAddrs, err := net.DefaultResolver.LookupIP(ctx, "ip6", host)
		if err != nil {
			return []netip.Addr{}, err
		} else if len(ipAddrs) == 0 {
			return []netip.Addr{}, ErrIPNotFound
		}

		addrs := make([]netip.Addr, 0, len(ipAddrs))
		for _, ipAddr := range ipAddrs {
			addrs = append(addrs, nnip.IpToAddr(ipAddr))
		}

		rand.Shuffle(len(addrs), func(i, j int) {
			addrs[i], addrs[j] = addrs[j], addrs[i]
		})
		return addrs, nil
	}
	return []netip.Addr{}, ErrIPNotFound
}

func ResolveAllIPv4WithResolver(host string, r Resolver) ([]netip.Addr, error) {
	if node := DefaultHosts.Search(host); node != nil {
		if ip := node.Data(); ip.Is4() {
			return []netip.Addr{node.Data()}, nil
		}
	}

	ip, err := netip.ParseAddr(host)
	if err == nil {
		if ip.Is4() || ip.Is4In6() {
			return []netip.Addr{ip}, nil
		}
		return []netip.Addr{}, ErrIPVersion
	}

	if r != nil {
		return r.ResolveAllIPv4(host)
	}

	if DefaultResolver == nil {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultDNSTimeout)
		defer cancel()
		ipAddrs, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return []netip.Addr{}, err
		} else if len(ipAddrs) == 0 {
			return []netip.Addr{}, ErrIPNotFound
		}

		addrs := make([]netip.Addr, 0, len(ipAddrs))
		for _, ipAddr := range ipAddrs {
			addrs = append(addrs, nnip.IpToAddr(ipAddr))
		}

		rand.Shuffle(len(addrs), func(i, j int) {
			addrs[i], addrs[j] = addrs[j], addrs[i]
		})
		return addrs, nil
	}
	return []netip.Addr{}, ErrIPNotFound
}

func ResolveAllIPWithResolver(host string, r Resolver) ([]netip.Addr, error) {
	if node := DefaultHosts.Search(host); node != nil {
		return []netip.Addr{node.Data()}, nil
	}

	ip, err := netip.ParseAddr(host)
	if err == nil {
		return []netip.Addr{ip}, nil
	}

	if r != nil {
		if DisableIPv6 {
			return r.ResolveAllIPv4(host)
		}

		return r.ResolveAllIP(host)
	} else if DisableIPv6 {
		return ResolveAllIPv4(host)
	}

	if DefaultResolver == nil {
		ipAddrs, err := net.DefaultResolver.LookupIP(context.Background(), "ip", host)
		if err != nil {
			return []netip.Addr{}, err
		} else if len(ipAddrs) == 0 {
			return []netip.Addr{}, ErrIPNotFound
		}
		addrs := make([]netip.Addr, 0, len(ipAddrs))
		for _, ipAddr := range ipAddrs {
			addrs = append(addrs, nnip.IpToAddr(ipAddr))
		}

		rand.Shuffle(len(addrs), func(i, j int) {
			addrs[i], addrs[j] = addrs[j], addrs[i]
		})
		return addrs, nil
	}
	return []netip.Addr{}, ErrIPNotFound
}

func ResolveAllIP(host string) ([]netip.Addr, error) {
	return ResolveAllIPWithResolver(host, DefaultResolver)
}

func ResolveAllIPv4(host string) ([]netip.Addr, error) {
	return ResolveAllIPv4WithResolver(host, DefaultResolver)
}

func ResolveAllIPv6(host string) ([]netip.Addr, error) {
	return ResolveAllIPv6WithResolver(host, DefaultResolver)
}

func ResolveAllIPv6ProxyServerHost(host string) ([]netip.Addr, error) {
	if ProxyServerHostResolver != nil {
		return ResolveAllIPv6WithResolver(host, ProxyServerHostResolver)
	}
	return ResolveAllIPv6(host)
}

func ResolveAllIPv4ProxyServerHost(host string) ([]netip.Addr, error) {
	if ProxyServerHostResolver != nil {
		return ResolveAllIPv4WithResolver(host, ProxyServerHostResolver)
	}
	return ResolveAllIPv4(host)
}

func ResolveAllIPProxyServerHost(host string) ([]netip.Addr, error) {
	if ProxyServerHostResolver != nil {
		return ResolveAllIPWithResolver(host, ProxyServerHostResolver)
	}
	return ResolveAllIP(host)
}
