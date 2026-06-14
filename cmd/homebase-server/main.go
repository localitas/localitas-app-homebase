package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	homebase "github.com/localitas/localitas-app-homebase"
	"github.com/localitas/localitas-go"
	"github.com/urfave/cli/v3"
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
	app := &cli.Command{
		Name:    "homebase-server",
		Usage:   "Homebase IoT control panel",
		Version: version,
		Commands: []*cli.Command{
			serveCommand(),
			migrateCommand(),
		},
		DefaultCommand: "serve",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return serveAction(ctx, cmd)
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func commonFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "listen", Value: ":0", Usage: "listen address"},
		&cli.StringFlag{Name: "core-url", Value: client.DefaultCoreURL(), Usage: "base URL of the Localitas core API"},
		&cli.StringFlag{Name: "base-path", Value: "/", Usage: "URL prefix for <base href>"},
		&cli.StringFlag{Name: "token", Value: envOrFileToken(), Usage: "bearer token for API calls"},
		&cli.StringFlag{Name: "sidecar-url", Value: "http://localhost:9222", Usage: "URL of the Matter sidecar"},
		&cli.StringFlag{Name: "hap-pin", Value: "00102003", Usage: "HomeKit pairing PIN"},
		&cli.StringFlag{Name: "hap-storage", Value: os.ExpandEnv("$HOME/.localitas/homebase/hap"), Usage: "HAP persistent storage directory"},
	}
}

func newClient(cmd *cli.Command) *client.Client {
	c := client.New(cmd.String("core-url"))
	if t := cmd.String("token"); t != "" {
		c = c.WithToken(t)
	}
	return c
}

func serveCommand() *cli.Command {
	return &cli.Command{
		Name:   "serve",
		Usage:  "Start the Homebase server",
		Flags:  commonFlags(),
		Action: serveAction,
	}
}

func serveAction(ctx context.Context, cmd *cli.Command) error {
	coreURL := cmd.String("core-url")
	basePath := cmd.String("base-path")
	token := cmd.String("token")
	sidecarURL := cmd.String("sidecar-url")
	hapPin := cmd.String("hap-pin")
	hapStorage := cmd.String("hap-storage")
	c := newClient(cmd)

	a := homebase.New(c, basePath, sidecarURL)

	dbID, err := a.Install(ctx)
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	log.Printf("Homebase database ready: %s", dbID)

	if err := a.InitStore(coreURL, dbID, token); err != nil {
		return fmt.Errorf("init store: %w", err)
	}
	defer a.Store.Close()

	hapBridge := homebase.NewHAPBridge(a.Sidecar, a.Store, hapPin, hapStorage)
	a.HAP = hapBridge

	hapCtx, hapCancel := context.WithCancel(ctx)
	defer hapCancel()
	if err := hapBridge.Start(hapCtx); err != nil {
		log.Printf("HAP bridge failed to start: %v", err)
	} else {
		log.Printf("HAP bridge started (PIN: %s)", hapPin)
	}

	pluginDiscovery := homebase.NewPluginDiscovery(a.Store, hapBridge)
	a.Plugins = pluginDiscovery
	pluginDiscovery.Start(hapCtx)
	log.Printf("Plugin discovery started")

	mux := http.NewServeMux()
	a.RegisterRoutes(mux)
	mux.HandleFunc("GET /health.json", homebase.HandleHealth)

	ln, err := net.Listen("tcp", cmd.String("listen"))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
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

	return http.Serve(ln, mux)
}

func migrateCommand() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Run database migrations without starting the server",
		Flags: commonFlags(),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			c := newClient(cmd)
			a := homebase.New(c, "/", "http://localhost:9222")
			dbID, err := a.Install(ctx)
			if err != nil {
				return fmt.Errorf("migrate: %w", err)
			}
			log.Printf("Homebase migrations complete (database: %s)", dbID)
			return nil
		},
	}
}
