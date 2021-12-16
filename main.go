package main

import (
	"fmt"
	"github.com/birneee/hquic-proxy-go/proxy"
	"github.com/urfave/cli/v2"
	"net"
	"os"
	"qperf-go/client"
	"qperf-go/common"
	"qperf-go/server"
	"time"
)

const defaultProxyControlPort = 18081
const defaultProxyTLSCertificateFile = "proxy.crt"
const defaultProxyTLSKeyFile = "proxy.key"

func main() {
	app := &cli.App{
		Name:  "qperf",
		Usage: "TODO",
		Commands: []*cli.Command{
			{
				Name:  "proxy",
				Usage: "run in proxy mode",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "addr",
						Usage: "address of the proxy to listen on",
						Value: "0.0.0.0",
					},
					&cli.UintFlag{
						Name:  "port",
						Usage: "port of the proxy to listen on, for control connections",
						Value: defaultProxyControlPort,
					},
					&cli.StringFlag{
						Name:  "tls-cert",
						Usage: "certificate file to use",
						Value: defaultProxyTLSCertificateFile,
					},
					&cli.StringFlag{
						Name:  "tls-key",
						Usage: "key file to use",
						Value: defaultProxyTLSKeyFile,
					},
					&cli.StringFlag{
						Name:  "next-proxy",
						Usage: "the additional, server-side proxy to use, in the form \"host:port\", default port 18081 if not specified",
					},
					&cli.StringFlag{
						Name:  "next-proxy-cert",
						Usage: "certificate file to trust the next proxy",
						Value: "proxy.crt",
					},
					&cli.StringFlag{
						Name:  "initial-congestion-window",
						Usage: "the initial congestion window to use, in bytes",
						Value: "39424B",
					},
					//TODO make name and description more clear
					&cli.StringFlag{
						Name:  "client-side-initial-receive-window",
						Usage: "overwrite the initial receive window on the client side proxy connection, instead of using the one from the handover state",
					},
					//TODO make name and description more clear
					&cli.StringFlag{
						Name:  "server-side-initial-receive-window",
						Usage: "overwrite the initial receive window on the server side proxy connection, instead of using the one from the handover state",
					},
					&cli.StringFlag{
						Name:  "server-side-max-receive-window",
						Usage: "overwrite the maximum receive window on the server side proxy connection, instead of using the one from the handover state",
					},
				},
				Action: func(c *cli.Context) error {
					var nextProxyAddr *net.UDPAddr
					if c.IsSet("next-proxy") {
						var err error
						nextProxyAddr, err = common.ParseResolveHost(c.String("next-proxy"), common.DefaultProxyControlPort)
						if err != nil {
							panic(err)
						}
					}
					initialCongestionWindow, err := common.ParseByteCountWithUnit(c.String("initial-congestion-window"))
					if err != nil {
						return fmt.Errorf("failed to parse initial-congestion-window: %w", err)
					}
					var clientSideInitialReceiveWindow uint64
					if c.IsSet("client-side-initial-receive-window") {
						clientSideInitialReceiveWindow, err = common.ParseByteCountWithUnit(c.String("client-side-initial-receive-window"))
						if err != nil {
							return fmt.Errorf("failed to parse client-side-initial-receive-window: %w", err)
						}
					}
					var serverSideInitialReceiveWindow uint64
					if c.IsSet("server-side-initial-receive-window") {
						serverSideInitialReceiveWindow, err = common.ParseByteCountWithUnit(c.String("server-side-initial-receive-window"))
						if err != nil {
							return fmt.Errorf("failed to parse server-side-initial-receive-window: %w", err)
						}
					}
					var serverSideMaxReceiveWindow uint64
					if c.IsSet("server-side-max-receive-window") {
						serverSideMaxReceiveWindow, err = common.ParseByteCountWithUnit(c.String("server-side-max-receive-window"))
						if err != nil {
							return fmt.Errorf("failed to parse server-side-max-receive-window: %w", err)
						}
					}
					proxy.RunProxy(
						net.UDPAddr{
							IP:   net.ParseIP(c.String("addr")),
							Port: c.Int("port"),
						},
						c.String("tls-cert"),
						c.String("tls-key"),
						nextProxyAddr,
						c.String("next-proxy-cert"),
						uint32(initialCongestionWindow),
						clientSideInitialReceiveWindow,
						serverSideInitialReceiveWindow,
						serverSideMaxReceiveWindow,
					)
					return nil
				},
			},
			{
				Name:  "client",
				Usage: "run in client mode",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "addr",
						Usage:    "address to connect to, in the form \"host:port\", default port 18080 if not specified",
						Required: true,
					},
					&cli.BoolFlag{
						Name:  "ttfb",
						Usage: "measure time for connection establishment and first byte only",
					},
					&cli.BoolFlag{
						Name:  "print-raw",
						Usage: "output raw statistics, don't calculate metric prefixes",
					},
					&cli.BoolFlag{
						Name:  "qlog",
						Usage: "create qlog file",
					},
					&cli.UintFlag{
						Name:  "migrate",
						Usage: "seconds after which the udp socket is migrated",
					},
					&cli.StringFlag{
						Name:  "proxy",
						Usage: "the proxy to use, in the form \"host:port\", default port 18081 if not specified",
					},
					&cli.UintFlag{
						Name:  "t",
						Usage: "run for this many seconds",
						Value: 10,
					},
					&cli.StringFlag{
						Name:  "tls-cert",
						Usage: "certificate file to trust the server",
						Value: "server.crt",
					},
					&cli.StringFlag{
						Name:  "tls-proxy-cert",
						Usage: "certificate file to trust the proxy",
						Value: "proxy.crt",
					},
					&cli.StringFlag{
						Name:  "initial-congestion-window",
						Usage: "the initial congestion window to use, in bytes",
						Value: "39424B",
					},
					&cli.StringFlag{
						Name:  "initial-receive-window",
						Usage: "the initial stream-level receive window, in bytes (the connection-level window is 1.5 times higher)",
						Value: "512KiB",
					},
					&cli.StringFlag{
						Name:  "max-receive-window",
						Usage: "the maximum stream-level receive window, in bytes (the connection-level window is 1.5 times higher)",
						Value: "6MiB",
					},
				},
				Action: func(c *cli.Context) error {
					var proxyAddr *net.UDPAddr
					if c.IsSet("proxy") {
						var err error
						proxyAddr, err = common.ParseResolveHost(c.String("proxy"), common.DefaultProxyControlPort)
						if err != nil {
							panic(err)
						}
					}
					serverAddr, err := common.ParseResolveHost(c.String("addr"), common.DefaultQperfServerPort)
					if err != nil {
						println("invalid server address")
						panic(err)
					}
					initialCongestionWindow, err := common.ParseByteCountWithUnit(c.String("initial-congestion-window"))
					if err != nil {
						return fmt.Errorf("failed to parse initial-congestion-window: %w", err)
					}
					initialReceiveWindow, err := common.ParseByteCountWithUnit(c.String("initial-receive-window"))
					if err != nil {
						return fmt.Errorf("failed to parse receive-window: %w", err)
					}
					maxReceiveWindow, err := common.ParseByteCountWithUnit(c.String("max-receive-window"))
					if err != nil {
						return fmt.Errorf("failed to parse receive-window: %w", err)
					}
					client.Run(
						*serverAddr,
						c.Bool("ttfb"),
						c.Bool("print-raw"),
						c.Bool("qlog"),
						time.Duration(c.Uint64("migrate"))*time.Second,
						proxyAddr,
						time.Duration(c.Uint("t"))*time.Second,
						c.String("tls-cert"),
						c.String("tls-proxy-cert"),
						uint32(initialCongestionWindow),
						initialReceiveWindow,
						maxReceiveWindow,
					)
					return nil
				},
			},
			{
				Name:  "server",
				Usage: "run in server mode",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "addr",
						Usage: "address to listen on",
						Value: "0.0.0.0",
					},
					&cli.UintFlag{
						Name:  "port",
						Usage: "port to listen on",
						Value: common.DefaultQperfServerPort,
					},
					&cli.BoolFlag{
						Name:  "qlog",
						Usage: "create qlog file",
					},
					&cli.UintFlag{
						Name:  "migrate",
						Usage: "seconds after which the udp socket is migrated",
					},
					&cli.StringFlag{
						Name:  "proxy",
						Usage: "the proxy to use, in the form \"host:port\", default port 18081 if not specified",
					},
					&cli.StringFlag{
						Name:  "tls-cert",
						Usage: "certificate file to use",
						Value: "server.crt",
					},
					&cli.StringFlag{
						Name:  "tls-key",
						Usage: "key file to use",
						Value: "server.key",
					},
					&cli.StringFlag{
						Name:  "tls-proxy-cert",
						Usage: "certificate file to trust",
						Value: "proxy.crt",
					},
					&cli.StringFlag{
						Name:  "initial-congestion-window",
						Usage: "the initial congestion window to use, in bytes",
						Value: "39424B",
					},
					&cli.StringFlag{
						Name:  "initial-receive-window",
						Usage: "the initial stream-level receive window, in bytes (the connection-level window is 1.5 times higher)",
						Value: "512KiB",
					},
					&cli.StringFlag{
						Name:  "max-receive-window",
						Usage: "the maximum stream-level receive window, in bytes (the connection-level window is 1.5 times higher)",
						Value: "6MiB",
					},
				},
				Action: func(c *cli.Context) error {
					var proxyAddr *net.UDPAddr
					if c.IsSet("proxy") {
						var err error
						proxyAddr, err = common.ParseResolveHost(c.String("proxy"), common.DefaultProxyControlPort)
						if err != nil {
							panic(err)
						}
					}
					initialCongestionWindow, err := common.ParseByteCountWithUnit(c.String("initial-congestion-window"))
					if err != nil {
						return fmt.Errorf("failed to parse initial-congestion-window: %w", err)
					}
					initialReceiveWindow, err := common.ParseByteCountWithUnit(c.String("initial-receive-window"))
					if err != nil {
						return fmt.Errorf("failed to parse receive-window: %w", err)
					}
					maxReceiveWindow, err := common.ParseByteCountWithUnit(c.String("max-receive-window"))
					if err != nil {
						return fmt.Errorf("failed to parse receive-window: %w", err)
					}
					server.Run(net.UDPAddr{
						IP:   net.ParseIP(c.String("addr")),
						Port: c.Int("port"),
					},
						c.Bool("qlog"),
						time.Duration(c.Uint64("migrate"))*time.Second,
						proxyAddr,
						c.String("tls-cert"),
						c.String("tls-key"),
						c.String("tls-proxy-cert"),
						uint32(initialCongestionWindow),
						initialReceiveWindow,
						maxReceiveWindow,
					)
					return nil
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}
