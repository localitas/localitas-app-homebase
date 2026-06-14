package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/localitas/localitas-app-homebase"
	"github.com/localitas/localitas-go"
)

var (
	version = "dev"
	commit  = "unknown"
)

func envOrFileToken() string {
	if t := os.Getenv("LOCALITAS_API_TOKEN"); t != "" {
		return t
	}
	return client.DefaultToken()
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("homebase-server %s (commit: %s)\n", version, commit)
		os.Exit(0)
	}

	var (
		listen     = flag.String("listen", ":0", "listen address (default :0 = random port)")
		coreURL    = flag.String("core-url", client.DefaultCoreURL(), "base URL of the Localitas core API")
		basePath   = flag.String("base-path", "/", "URL prefix for <base href>")
		token      = flag.String("token", envOrFileToken(), "bearer token for install + SQL driver")
		sidecarURL = flag.String("sidecar-url", "http://localhost:9222", "URL of the Matter sidecar")
		hapPin     = flag.String("hap-pin", "00102003", "HomeKit pairing PIN")
		hapStorage = flag.String("hap-storage", os.ExpandEnv("$HOME/.localitas/homebase/hap"), "HAP persistent storage directory")
	)
	flag.Parse()

	ctx := context.Background()
	c := client.New(*coreURL)
	if *token != "" {
		c = c.WithToken(*token)
	}

	app := homebase.New(c, *basePath, *sidecarURL)

	dbID, err := app.Install(ctx)
	if err != nil {
		log.Fatalf("install: %v", err)
	}
	log.Printf("Homebase database ready: %s", dbID)

	if err := app.InitStore(*coreURL, dbID, *token); err != nil {
		log.Fatalf("init store: %v", err)
	}
	defer app.Store.Close()

	hapBridge := homebase.NewHAPBridge(app.Sidecar, app.Store, *hapPin, *hapStorage)
	app.HAP = hapBridge

	hapCtx, hapCancel := context.WithCancel(ctx)
	defer hapCancel()
	if err := hapBridge.Start(hapCtx); err != nil {
		log.Printf("HAP bridge failed to start: %v", err)
	} else {
		log.Printf("HAP bridge started (PIN: %s)", *hapPin)
	}

	pluginDiscovery := homebase.NewPluginDiscovery(app.Store, hapBridge)
	app.Plugins = pluginDiscovery
	pluginDiscovery.Start(hapCtx)
	log.Printf("Plugin discovery started")

	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	mux.HandleFunc("GET /health.json", homebase.HandleHealth)

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	fmt.Printf("homebase-server listening on http://localhost:%d\n", addr.Port)

	selfURL := fmt.Sprintf("http://localhost:%d", addr.Port)
	if err := c.RegisterService(ctx, "homebase", selfURL); err != nil {
		log.Printf("service registry failed: %v", err)
	}

	shutdown, err := homebase.BroadcastMDNS(addr.Port, homebase.DefaultHealth.Name)
	if err != nil {
		log.Printf("mDNS broadcast failed: %v", err)
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutting down...")
		hapCancel()
		if shutdown != nil {
			shutdown()
		}
		os.Exit(0)
	}()

	if err := http.Serve(ln, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
