package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/aztecrabbit/libinject"
	"github.com/aztecrabbit/liblog"
	"github.com/aztecrabbit/libproxyrotator"
	"github.com/aztecrabbit/libredsocks"
	"github.com/aztecrabbit/libutils"
	"github.com/miekg/dns"
	"github.com/miekg/v2ray-core/proxy/vmess"
	"github.com/miekg/v2ray-core/proxy/vmess/outbound"
	"github.com/miekg/v2ray-core/proxy/vmess/protocol"
)

const (
	appName        = "Brainfuck Tunnel"
	appVersionName = "Psiphon Pro Go"
	appVersionCode = "1.3.210109"

	copyrightYear   = "2020"
	copyrightAuthor = "Aztec Rabbit"
)

var (
	InterruptHandler = new(libutils.InterruptHandler)
	Redsocks         = new(libredsocks.Redsocks)
	VmessClient      *outbound.Client
)

type Config struct {
	ProxyRotator *libproxyrotator.Config
	Inject       *libinject.Config
	Vmess        *VmessConfig
}

type VmessConfig struct {
	Address string
	Port    string
	Id      string
	AlterId int
	Security string
}

func init() {
	InterruptHandler.Handle = func() {
		libredsocks.Stop(Redsocks)
		liblog.LogKeyboardInterrupt()
	}
	InterruptHandler.Start()
}

func GetConfigPath(filename string) string {
	return libutils.GetConfigPath("brainfuck-psiphon-pro-go", filename)
}

func main() {
	liblog.Header(
		[]string{
			fmt.Sprintf("%s [%s Version. %s]", appName, appVersionName, appVersionCode),
			fmt.Sprintf("(c) %s %s.", copyrightYear, copyrightAuthor),
		},
		liblog.Colors["G1"],
	)

	config := new(Config)
	defaultConfig := new(Config)
	defaultConfig.ProxyRotator = libproxyrotator.DefaultConfig
	defaultConfig.Inject = libinject.DefaultConfig
	defaultConfig.Inject.Type = 2
	defaultConfig.Inject.Rules = map[string][]string{
		"akamai.net:80": []string{
			"video.iflix.com",
			"videocdn-2.iflix.com",
			"iflix-videocdn-p1.akamaized.net",
			"iflix-videocdn-p2.akamaized.net",
			"iflix-videocdn-p3.akamaized.net",
			"iflix-videocdn-p6.akamaized.net",
			"iflix-videocdn-p7.akamaized.net",
			"iflix-videocdn-p8.akamaized.net",
		},
	}
	defaultConfig.Inject.Payload = ""
	defaultConfig.Inject.Timeout = 5
	defaultConfig.Vmess = &VmessConfig{
		Address: "157.10.52.130",
		Port:    "80",
		Id:      "f8c182c3-b3ea-586c-9c29-8e7d664e4f16",
		AlterId: 2,
		Security: "auto",
	}

	libutils.JsonReadWrite(GetConfigPath("config.json"), config, defaultConfig)

	var flagRefresh = false
	var flagVerbose = false
	var flagFrontend string
	var flagWhitelist string

	flag.BoolVar(&flagRefresh, "refresh", flagRefresh, "Refresh Data")
	flag.BoolVar(&flagVerbose, "verbose", flagVerbose, "Verbose Log?")
	flag.StringVar(&flagFrontend, "f", flagFrontend, "-f frontend-domains (e.g. -f cdn.com,cdn.com:443)")
	flag.StringVar(&flagWhitelist, "w", flagWhitelist, "-w whitelist-request (e.g. -w akamai.net:80)")
	flag.IntVar(&config.Inject.MeekType, "mt", config.Inject.MeekType, "-mt meek type (0 and 1 for fastly)")
	flag.Parse()

	if flagRefresh {
		//libpsiphon.RemoveData()
	}

	if flagFrontend != "" || flagWhitelist != "" {
		if flagFrontend == "" {
			flagFrontend = "*"
		}
		if flagWhitelist == "" {
			flagWhitelist = "*:*"
		}

		config.Inject.Rules = map[string][]string{
			flagWhitelist: strings.Split(flagFrontend, ","),
		}
	}

	ProxyRotator := new(libproxyrotator.ProxyRotator)
	ProxyRotator.Config = config.ProxyRotator

	Inject := new(libinject.Inject)
	Inject.Redsocks = Redsocks
	Inject.Config = config.Inject

	go ProxyRotator.Start()
	go Inject.Start()

	time.Sleep(200 * time.Millisecond)

	liblog.LogInfo("Domain Fronting running on port "+Inject.Config.Port, "INFO", liblog.Colors["G1"])
	liblog.LogInfo("Proxy Rotator running on port "+ProxyRotator.Config.Port, "INFO", liblog.Colors["G1"])

	Redsocks.Config = libredsocks.DefaultConfig
	Redsocks.Config.LogOutput = GetConfigPath("redsocks.log")
	Redsocks.Config.ConfigOutput = GetConfigPath("redsocks.conf")
	Redsocks.Start()

	// Tích hợp Vmess
	vmessConfig := &vmess.Config{
		Addrs: []string{
			fmt.Sprintf("%s:%s", config.Vmess.Address, config.Vmess.Port),
		},
	}

	if config.Vmess.AlterId <= 0 {
		config.Vmess.AlterId = 1
	}
	v, err := protocol.NewVmessOutbound(config.Vmess.Id, config.Vmess.AlterId,
		config.Vmess.Security, vmessConfig)
	if err != nil {
		liblog.LogInfo(fmt.Sprintf("Error creating Vmess Outbound: %s", err), "INFO", liblog.Colors["R1"])
		return
	}
	VmessClient = outbound.NewClient(v, nil)

	go func() {
		dns.HandleFunc(":53", func(w dns.ResponseWriter, req *dns.Msg) {
			liblog.LogVerbose(fmt.Sprintf("dns req: %+v", req), "DEBUG", liblog.Colors["CC"])
			res := new(dns.Msg)
			res.SetReply(req)
			res.RecursionAvailable = false
			VmessClient.Transport(res, req)
			liblog.LogVerbose(fmt.Sprintf("dns res: %+v", res), "DEBUG", liblog.Colors["CC"])
			w.WriteMsg(res)
		})
		dns.ListenAndServe(":53", "udp", nil)
	}()

	InterruptHandler.Wait()
}

