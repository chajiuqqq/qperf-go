package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"qperf-go/common"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// Run server.
// if proxyAddr is nil, no proxy is used.
func Run(addr net.UDPAddr, createQLog bool, migrateAfter time.Duration, tlsServerCertFile string, tlsServerKeyFile string, initialCongestionWindow uint32, minCongestionWindow uint32, maxCongestionWindow uint32, initialReceiveWindow uint64, maxReceiveWindow uint64, noXse bool, logPrefix string, qlogPrefix string, http3enabled bool, www string) {

	logger := common.DefaultLogger.WithPrefix(logPrefix)

	// tracers := make([]logging.Tracer, 0)

	if createQLog {
		// tracers = append(tracers, common.NewQlogTracer(qlogPrefix, logger))
	}

	// TODO somehow associate it with the qperf session for logging

	// tracers = append(tracers, common.NewEventTracer(common.Handlers{
	// 	UpdatePath: func(odcid logging.ConnectionID, newRemote net.Addr) {
	// 		logger.Infof("migrated QUIC connection %s to %s", odcid.String(), newRemote)
	// 	},
	// 	StartedConnection: func(odcid logging.ConnectionID, local, remote net.Addr, srcConnID, destConnID logging.ConnectionID) {
	// 		logger.Infof("started QUIC connection %s", odcid.String())
	// 	},
	// 	ClosedConnection: func(odcid logging.ConnectionID, err error) {
	// 		logger.Infof("closed QUIC connection %s", odcid.String())
	// 	},
	// }))

	if initialReceiveWindow > maxReceiveWindow {
		maxReceiveWindow = initialReceiveWindow
	}

	if initialCongestionWindow < minCongestionWindow {
		initialCongestionWindow = minCongestionWindow
	}

	conf := quic.Config{
		// Tracer:                         logging.NewMultiplexedTracer(tracers...),
		// EnableActiveMigration:          true,
		// InitialCongestionWindow:        initialCongestionWindow,
		// MinCongestionWindow:            minCongestionWindow,
		// MaxCongestionWindow:            maxCongestionWindow,
		InitialStreamReceiveWindow: initialReceiveWindow,
		MaxStreamReceiveWindow:     maxReceiveWindow,
		// InitialConnectionReceiveWindow: uint64(float64(initialReceiveWindow) * quic.ConnectionFlowControlMultiplier),
		// MaxConnectionReceiveWindow:     uint64(float64(maxReceiveWindow) * quic.ConnectionFlowControlMultiplier),
		// TODO add option to disable mtu discovery
		// TODO add option to enable address prevalidation
	}

	// if noXse {
	// 	conf.ExtraStreamEncryption = quic.DisableExtraStreamEncryption
	// } else {
	// 	conf.ExtraStreamEncryption = quic.PreferExtraStreamEncryption
	// }

	// TODO make CLI option
	tlsCert, err := tls.LoadX509KeyPair(tlsServerCertFile, tlsServerKeyFile)
	if err != nil {
		panic(err)
	}

	tlsConf := tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"qperf"},
	}

	// http3
	if http3enabled {
		logger.Infof("http3 served on %s", addr.String())
		handler := setupHandler(www)
		server := http3.Server{
			Handler:    handler,
			Addr:       addr.String(),
			QuicConfig: &conf,
			TLSConfig:  &tlsConf,
		}
		err = server.ListenAndServe()
		if err != nil {
			panic(err)
		}
		return
	}

	listener, err := quic.ListenAddrEarly(addr.String(), &tlsConf, &conf)
	if err != nil {
		panic(err)
	}

	// print new reno as this is the only option in quic-go
	logger.Infof("starting server with pid %d, port %d, cc new reno", os.Getpid(), addr.Port)

	// migrate
	// if migrateAfter.Nanoseconds() != 0 {
	// 	go func() {
	// 		time.Sleep(migrateAfter)
	// 		addr, err := listener.MigrateUDPSocket()
	// 		if err != nil {
	// 			panic(err)
	// 		}
	// 		logger.Infof("migrated to %s", addr.String())
	// 	}()
	// }

	var nextConnectionId uint64 = 0

	for {
		quicConnection, err := listener.Accept(context.Background())
		if err != nil {
			panic(err)
		}

		qperfSession := &qperfServerSession{
			connection:   quicConnection,
			connectionID: nextConnectionId,
			logger:       logger.WithPrefix(fmt.Sprintf("connection %d", nextConnectionId)),
		}

		go qperfSession.run()
		nextConnectionId += 1
	}
}

// See https://en.wikipedia.org/wiki/Lehmer_random_number_generator
func generatePRData(l int) []byte {
	res := make([]byte, l)
	seed := uint64(1)
	for i := 0; i < l; i++ {
		seed = seed * 48271 % 2147483647
		res[i] = byte(seed)
	}
	return res
}

var html = template.Must(template.New("https").Parse(`
<html>
<head>
  <title>Https Test</title>
</head>
<body>
  <h1 style="color:red;">Img:{{ .filename }}</h1>
  <img src="www/{{ .filename }}"></img>
</body>
</html>
`))

func setupHandler(www string) http.Handler {

	r := gin.Default()
	r.SetHTMLTemplate(html)
	r.Static("/www", www)

	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "success",
		})
	})
	r.GET("/html/:filename", func(c *gin.Context) {
		c.HTML(http.StatusOK, "https", gin.H{
			"status":   "success",
			"filename": c.Param("filename"),
		})
	})
	return r

	// mux := http.NewServeMux()

	// if len(www) > 0 {
	// 	mux.Handle("/", http.FileServer(http.Dir(www)))
	// } else {
	// 	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	// 		fmt.Printf("%#v\n", r)
	// 		const maxSize = 1 << 30 // 1 GB
	// 		num, err := strconv.ParseInt(strings.ReplaceAll(r.RequestURI, "/", ""), 10, 64)
	// 		if err != nil || num <= 0 || num > maxSize {
	// 			w.WriteHeader(400)
	// 			return
	// 		}
	// 		w.Write(generatePRData(int(num)))
	// 	})
	// }

	// mux.HandleFunc("/demo/tile", func(w http.ResponseWriter, r *http.Request) {
	// 	// Small 40x40 png
	// 	w.Write([]byte{
	// 		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	// 		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00, 0x28,
	// 		0x01, 0x03, 0x00, 0x00, 0x00, 0xb6, 0x30, 0x2a, 0x2e, 0x00, 0x00, 0x00,
	// 		0x03, 0x50, 0x4c, 0x54, 0x45, 0x5a, 0xc3, 0x5a, 0xad, 0x38, 0xaa, 0xdb,
	// 		0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41, 0x54, 0x78, 0x01, 0x63, 0x18,
	// 		0x61, 0x00, 0x00, 0x00, 0xf0, 0x00, 0x01, 0xe2, 0xb8, 0x75, 0x22, 0x00,
	// 		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	// 	})
	// })

	// mux.HandleFunc("/demo/tiles", func(w http.ResponseWriter, r *http.Request) {
	// 	io.WriteString(w, "<html><head><style>img{width:40px;height:40px;}</style></head><body>")
	// 	for i := 0; i < 200; i++ {
	// 		fmt.Fprintf(w, `<img src="/demo/tile?cachebust=%d">`, i)
	// 	}
	// 	io.WriteString(w, "</body></html>")
	// })

	// mux.HandleFunc("/demo/echo", func(w http.ResponseWriter, r *http.Request) {
	// 	body, err := io.ReadAll(r.Body)
	// 	if err != nil {
	// 		fmt.Printf("error reading body while handling /echo: %s\n", err.Error())
	// 	}
	// 	w.Write(body)
	// })
	// return mux
}
