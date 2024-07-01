package main

import (
    "flag"
    "fmt"
    "os"
    "runtime"
    "strings"
    "time"

    "github.com/aztecrabbit/brainfuck-psiphon-pro-go/src/libpsiphon"
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
    PsiphonCore  int
    Psiphon      *libpsiphon.Config
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
        libpsiphon.Stop()
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
    defaultConfig.PsiphonCore = 4
    defaultConfig.Psiphon = libpsiphon.DefaultConfig
    defaultConfig.Vmess = &VmessConfig{
        Address: "157.10.52.130",
        Port:    "80",
        Id:      "f8c182c3-b3ea-586c-9c29-8e7d664e4f16",
        AlterId: 2,
        Security: "auto",
    }

    if runtime.GOOS == "windows" {
        defaultConfig.Psiphon.CoreName += ".exe"
    }

    libutils.JsonReadWrite(GetConfigPath("config.json"), config, defaultConfig)

    var flagPro = true
    var flagRefresh = false
    var flagVerbose = false
    var flagFrontend string
    var flagWhitelist string

    flag.BoolVar(&flagPro, "pro", flagPro, "Pro Version?")
    flag.BoolVar(&flagRefresh, "refresh", flagRefresh, "Refresh Data")
    flag.BoolVar(&flagVerbose, "verbose", flagVerbose, "Verbose Log?")
    flag.StringVar(&flagFrontend, "f", flagFrontend, "-f frontend-domains (e.g. -f cdn.com,cdn.com:443)")
    flag.StringVar(&flagWhitelist, "w", flagWhitelist, "-w whitelist-request (e.g. -w akamai.net:80)")
    flag.IntVar(&config.Inject.MeekType, "mt", config.Inject.MeekType, "-mt meek type (0 and 1 for fastly)")
    flag.IntVar(&config.PsiphonCore, "c", config.PsiphonCore, "-c core (e.g. -c 4) (1 for Pro Version)")
    flag.StringVar(&config.Psiphon.Region, "r", config.Psiphon.Region, "-r region (e.g. -r sg)")
    flag.IntVar(&config.Psiphon.Tunnel, "t", config.Psiphon.Tunnel, "-t tunnel (e.g. -t 4) (1 for Reconnect Version)")
    flag.IntVar(&config.Psiphon.TunnelWorkers, "tw", config.Psiphon.TunnelWorkers, "-tw tunnel-workers (e.g. -tw 6) (8 for Pro Version)")
    flag.IntVar(&config.Psiphon.KuotaDataLimit, "l", config.Psiphon.KuotaDataLimit, "-l limit (in MB) (e.g. -l 4) (0 for Pro Version (unlimited))")
    flag.Parse()

    if !flagPro {
        config.Psiphon.Authorizations = make([]string, 0)
    }

    if flagRefresh {
        libpsiphon.RemoveData()
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

    if _, err := os.Stat(libutils.RealPath(config.Psiphon.CoreName)); os.IsNotExist(err) {
        liblog.LogInfo(
            fmt.Sprintf(
                "Exception:\n\n"+
                    "|	 File '%s' not exist!\n"+
                    "|	 Exiting...\n"+
                    "|\n",
                config.Psiphon.CoreName,
            ),
            "INFO", liblog.Colors["R1"],
        )
        return
    }

    Redsocks.Config = libredsocks.DefaultConfig
    Redsocks.Config.LogOutput = GetConfigPath("redsocks.log")
    Redsocks.Config.ConfigOutput = GetConfigPath("redsocks.conf")
    Redsocks.Start()

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
            vmessClient.Transport(res, req)
            liblog.LogVerbose(fmt.Sprintf("dns res: %+v", res), "DEBUG", liblog.Colors["CC"])
            w.WriteMsg(res)
        })
        dns.ListenAndServe(":53", "udp", nil)
    }()

    for i := 1; i <= config.PsiphonCore; i++ {
        Psiphon := new(libpsiphon.Psiphon)
        Psiphon.ProxyRotator = ProxyRotator
        Psiphon.Config = config.Psiphon
        Psiphon.ProxyPort = "53"
        Psiphon.KuotaData = libpsiphon.DefaultKuotaData
        Psiphon.ListenPort = libutils.Atoi(ProxyRotator.Config.Port) + i
        Psiphon.Verbose = flagVerbose

        go Psiphon.Start()
    }

    InterruptHandler.Wait()
}

