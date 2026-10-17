package homebase

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/localitas/localitas-go"
)

var logger = slog.Default().With("component", "homebase")

type App struct {
	Store      *Store
	BasePath   string
	Sidecar    *SidecarClient
	SidecarURL string
	HAP        *HAPBridge
	Plugins    *PluginDiscovery
	client     *client.Client
}

func New(c *client.Client, basePath, sidecarURL string) *App {
	if basePath == "" {
		basePath = "/"
	}
	return &App{
		BasePath:   basePath,
		Sidecar:    NewSidecarClient(sidecarURL),
		SidecarURL: sidecarURL,
		client:     c,
	}
}

func (a *App) InitStore(coreURL, dbID, token string) error {
	store, err := OpenStore(coreURL, dbID, token)
	if err != nil {
		return err
	}
	a.Store = store
	return nil
}

func (a *App) Install(ctx context.Context) (string, error) {
	for attempt := 1; ; attempt++ {
		db, err := a.client.CreateSystemDatabase(ctx, DatabaseName)
		if err != nil {
			logger.Warn("install attempt failed, retrying", "attempt", attempt, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if err := applyEmbeddedMigrations(ctx, a.client, db.ID); err != nil {
			logger.Warn("migrations attempt failed, retrying", "attempt", attempt, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		return db.ID, nil
	}
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(TemplatesFS, "templates/index.html")
	if err != nil {
		logger.Error("index template error", "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	data := map[string]string{
		"BasePath": a.BasePath,
	}
	if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		logger.Error("index render error", "error", err)
	}
}

func (a *App) RegisterRoutes(mux *http.ServeMux) {
	h := &handler{app: a}
	p := &partialHandler{app: a}

	mux.HandleFunc("GET /{$}", a.handleIndex)
	mux.HandleFunc("GET /swagger.json", HandleSwagger)
	mux.HandleFunc("GET /help.md", handleHelpMarkdown)

	// Every API and partial route acts for the user core vouches for;
	// AuthMiddleware also sets the scope the write and admin routes check.

	// Read endpoints (any verified user)
	mux.Handle("GET /api/devices", client.AuthMiddleware(http.HandlerFunc(h.handleListDevices)))
	mux.Handle("GET /api/devices/{id}", client.AuthMiddleware(http.HandlerFunc(h.handleGetDevice)))
	mux.Handle("GET /api/devices/{id}/state", client.AuthMiddleware(http.HandlerFunc(h.handleGetDeviceState)))
	mux.Handle("GET /api/devices/{id}/snapshot", client.AuthMiddleware(http.HandlerFunc(h.handleGetSnapshot)))
	mux.Handle("GET /api/rooms", client.AuthMiddleware(http.HandlerFunc(h.handleListRooms)))
	mux.Handle("GET /api/clusters", client.AuthMiddleware(http.HandlerFunc(h.handleListClusters)))
	mux.Handle("GET /api/plugins", client.AuthMiddleware(http.HandlerFunc(h.handleListPlugins)))
	mux.Handle("GET /api/sidecar/health", client.AuthMiddleware(http.HandlerFunc(h.handleSidecarHealth)))

	// Write endpoints (require write scope)
	mux.Handle("POST /api/devices", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeWrite, h.handleCommission))))
	mux.Handle("PUT /api/devices/{id}", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeWrite, h.handleUpdateDevice))))
	mux.Handle("DELETE /api/devices/{id}", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeWrite, h.handleDecommission))))
	mux.Handle("POST /api/devices/{id}/command", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeWrite, h.handleCommand))))
	mux.Handle("POST /api/virtual-devices", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeWrite, h.handleCreateVirtual))))

	// Admin endpoints (require admin scope)
	mux.Handle("GET /api/plugins/{name}/credential", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeAdmin, h.handleGetPluginCredential))))
	mux.Handle("PUT /api/plugins/{name}/credential", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeAdmin, h.handleSetPluginCredential))))
	mux.Handle("DELETE /api/plugins/{name}/credential", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeAdmin, h.handleDeletePluginCredential))))

	// HTMX partials (read)
	mux.Handle("GET /partials/sidebar", client.AuthMiddleware(http.HandlerFunc(p.handleSidebar)))
	mux.Handle("GET /partials/empty", client.AuthMiddleware(http.HandlerFunc(p.handleEmpty)))
	mux.Handle("GET /partials/device/{id}", client.AuthMiddleware(http.HandlerFunc(p.handleDeviceDetail)))
	mux.Handle("GET /partials/add-form", client.AuthMiddleware(http.HandlerFunc(p.handleAddForm)))
	mux.Handle("GET /partials/plugin-settings", client.AuthMiddleware(http.HandlerFunc(p.handlePluginSettings)))
	mux.Handle("GET /partials/sidecar-status", client.AuthMiddleware(http.HandlerFunc(p.handleSidecarStatus)))

	// HTMX partials (write)
	mux.Handle("PUT /partials/device/{id}", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeWrite, p.handleUpdateDevice))))
	mux.Handle("DELETE /partials/device/{id}", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeWrite, p.handleDeleteDevice))))
	mux.Handle("POST /partials/commission", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeWrite, p.handleCommission))))
	mux.Handle("POST /partials/create-virtual", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeWrite, p.handleCreateVirtual))))

	// HTMX partials (admin)
	mux.Handle("PUT /partials/plugin-credential/{name}", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeAdmin, p.handleSetPluginCredential))))
	mux.Handle("DELETE /partials/plugin-credential/{name}", client.AuthMiddleware(http.HandlerFunc(client.RequireScopeFunc(client.ScopeAdmin, p.handleDeletePluginCredential))))
}
