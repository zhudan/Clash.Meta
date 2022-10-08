package logic

import (
	"fmt"
	C "github.com/Dreamacro/clash/constant"
	RC "github.com/Dreamacro/clash/rules/common"
	RP "github.com/Dreamacro/clash/rules/provider"
	"github.com/stretchr/testify/assert"
	"testing"
)

func ParseRule(tp, payload, target string, params []string, subRules *map[string][]C.Rule) (parsed C.Rule, parseErr error) {
	switch tp {
	case "DOMAIN":
		parsed = RC.NewDomain(payload, target)
	case "DOMAIN-SUFFIX":
		parsed = RC.NewDomainSuffix(payload, target)
	case "DOMAIN-KEYWORD":
		parsed = RC.NewDomainKeyword(payload, target)
	case "GEOSITE":
		parsed, parseErr = RC.NewGEOSITE(payload, target)
	case "GEOIP":
		noResolve := RC.HasNoResolve(params)
		parsed, parseErr = RC.NewGEOIP(payload, target, noResolve)
	case "IP-CIDR", "IP-CIDR6":
		noResolve := RC.HasNoResolve(params)
		parsed, parseErr = RC.NewIPCIDR(payload, target, RC.WithIPCIDRNoResolve(noResolve))
	case "SRC-IP-CIDR":
		parsed, parseErr = RC.NewIPCIDR(payload, target, RC.WithIPCIDRSourceIP(true), RC.WithIPCIDRNoResolve(true))
	case "IP-SUFFIX":
		noResolve := RC.HasNoResolve(params)
		parsed, parseErr = RC.NewIPSuffix(payload, target, false, noResolve)
	case "SRC-IP-SUFFIX":
		parsed, parseErr = RC.NewIPSuffix(payload, target, true, true)
	case "SRC-PORT":
		parsed, parseErr = RC.NewPort(payload, target, true)
	case "DST-PORT":
		parsed, parseErr = RC.NewPort(payload, target, false)
	case "PROCESS-NAME":
		parsed, parseErr = RC.NewProcess(payload, target, true)
	case "PROCESS-PATH":
		parsed, parseErr = RC.NewProcess(payload, target, false)
	case "NETWORK":
		parsed, parseErr = RC.NewNetworkType(payload, target)
	case "UID":
		parsed, parseErr = RC.NewUid(payload, target)
	case "IN-TYPE":
		parsed, parseErr = RC.NewInType(payload, target)
	case "SUB-RULE":
		parsed, parseErr = NewSubRule(payload, target, subRules, ParseRule)
	case "AND":
		parsed, parseErr = NewAND(payload, target, ParseRule)
	case "OR":
		parsed, parseErr = NewOR(payload, target, ParseRule)
	case "NOT":
		parsed, parseErr = NewNOT(payload, target, ParseRule)
	case "RULE-SET":
		noResolve := RC.HasNoResolve(params)
		parsed, parseErr = RP.NewRuleSet(payload, target, noResolve)
	case "MATCH":
		parsed = RC.NewMatch(target)
		parseErr = nil
	default:
		parseErr = fmt.Errorf("unsupported rule type %s", tp)
	}

	return
}

func TestAND(t *testing.T) {
	and, err := NewAND("((DOMAIN,baidu.com),(NETWORK,TCP),(DST-PORT,10001-65535))", "DIRECT", ParseRule)
	assert.Equal(t, nil, err)
	assert.Equal(t, "DIRECT", and.adapter)
	assert.Equal(t, false, and.ShouldResolveIP())
	m, _ := and.Match(&C.Metadata{
		Host:     "baidu.com",
		AddrType: C.AtypDomainName,
		NetWork:  C.TCP,
		DstPort:  "20000",
	})
	assert.Equal(t, true, m)

	and, err = NewAND("(DOMAIN,baidu.com),(NETWORK,TCP),(DST-PORT,10001-65535))", "DIRECT", ParseRule)
	assert.NotEqual(t, nil, err)

	and, err = NewAND("((AND,(DOMAIN,baidu.com),(NETWORK,TCP)),(NETWORK,TCP),(DST-PORT,10001-65535))", "DIRECT", ParseRule)
	assert.Equal(t, nil, err)
}

func TestNOT(t *testing.T) {
	not, err := NewNOT("((DST-PORT,6000-6500))", "REJECT", ParseRule)
	assert.Equal(t, nil, err)
	m, _ := not.Match(&C.Metadata{
		DstPort: "6100",
	})
	assert.Equal(t, false, m)

	_, err = NewNOT("((DST-PORT,5600-6666),(DOMAIN,baidu.com))", "DIRECT", ParseRule)
	assert.NotEqual(t, nil, err)

	_, err = NewNOT("(())", "DIRECT", ParseRule)
	assert.NotEqual(t, nil, err)
}

func TestOR(t *testing.T) {
	or, err := NewOR("((DOMAIN,baidu.com),(NETWORK,TCP),(DST-PORT,10001-65535))", "DIRECT", ParseRule)
	assert.Equal(t, nil, err)
	m, _ := or.Match(&C.Metadata{
		NetWork: C.TCP,
	})
	assert.Equal(t, true, m)
	assert.Equal(t, false, or.ShouldResolveIP())
}
