module github.com/Dreamacro/clash

go 1.19

require (
	github.com/cilium/ebpf v0.9.3
	github.com/coreos/go-iptables v0.6.0
	github.com/database64128/tfo-go v1.1.2
	github.com/dlclark/regexp2 v1.7.0
	github.com/go-chi/chi/v5 v5.0.7
	github.com/go-chi/cors v1.2.1
	github.com/go-chi/render v1.0.2
	github.com/gofrs/uuid v4.3.0+incompatible
	github.com/google/gopacket v1.1.19
	github.com/gorilla/websocket v1.5.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/insomniacslk/dhcp v0.0.0-20221001123530-5308ebe5334c
	github.com/lucas-clemente/quic-go v0.29.1
	github.com/lunixbochs/struc v0.0.0-20200707160740-784aaebc1d40
	github.com/metacubex/sing-wireguard v0.0.0-20221109114053-16c22adda03c
	github.com/miekg/dns v1.1.50
	github.com/oschwald/geoip2-golang v1.8.0
	github.com/sagernet/netlink v0.0.0-20220905062125-8043b4a9aa97
	github.com/sagernet/sing v0.0.0-20221008120626-60a9910eefe4
	github.com/sagernet/sing-shadowsocks v0.0.0-20220819002358-7461bb09a8f6
	github.com/sagernet/sing-tun v0.0.0-20221012082254-488c3b75f6fd
	github.com/sagernet/sing-vmess v0.0.0-20221109021549-b446d5bdddf0
	github.com/sagernet/wireguard-go v0.0.0-20221108054404-7c2acadba17c
	github.com/sirupsen/logrus v1.9.0
	github.com/stretchr/testify v1.8.0
	github.com/xtls/go v0.0.0-20220914232946-0441cf4cf837
	go.etcd.io/bbolt v1.3.6
	go.uber.org/atomic v1.10.0
	go.uber.org/automaxprocs v1.5.1
	golang.org/x/crypto v0.1.0
	golang.org/x/exp v0.0.0-20220930202632-ec3f01382ef9
	golang.org/x/net v0.1.0
	golang.org/x/sync v0.0.0-20220929204114-8fcdb60fdcc0
	golang.org/x/sys v0.1.0
	google.golang.org/protobuf v1.28.1
	gopkg.in/yaml.v3 v3.0.1

)

replace github.com/lucas-clemente/quic-go => github.com/HyNetwork/quic-go v0.30.1-0.20221105180419-83715d7269a8

replace github.com/sagernet/sing-tun => github.com/MetaCubeX/sing-tun v0.0.0-20221105124245-542e9b56a6dc

require (
	github.com/ajg/form v1.5.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/pprof v0.0.0-20210407192527-94a9f03dee38 // indirect
	github.com/klauspost/cpuid/v2 v2.1.1 // indirect
	github.com/marten-seemann/qpack v0.3.0 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.3 // indirect
	github.com/marten-seemann/qtls-go1-19 v0.1.1 // indirect
	github.com/onsi/ginkgo/v2 v2.2.0 // indirect
	github.com/oschwald/maxminddb-golang v1.10.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sagernet/abx-go v0.0.0-20220819185957-dba1257d738e // indirect
	github.com/sagernet/go-tun2socks v1.16.12-0.20220818015926-16cb67876a61 // indirect
	github.com/u-root/uio v0.0.0-20220204230159-dac05f7d2cb4 // indirect
	github.com/vishvananda/netns v0.0.0-20220913150850-18c4f4234207 // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/text v0.4.0 // indirect
	golang.org/x/time v0.0.0-20220922220347-f3bd1da661af // indirect
	golang.org/x/tools v0.1.12 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gvisor.dev/gvisor v0.0.0-20220901235040-6ca97ef2ce1c // indirect
	lukechampine.com/blake3 v1.1.7 // indirect
)
