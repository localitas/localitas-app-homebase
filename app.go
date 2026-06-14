package homebase

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/localitas/localitas-go"
)

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
			log.Printf("install: attempt %d failed (retrying): %v", attempt, err)
			time.Sleep(2 * time.Second)
			continue
		}
		if err := applyEmbeddedMigrations(ctx, a.client, db.ID); err != nil {
			log.Printf("install: migrations attempt %d failed (retrying): %v", attempt, err)
			time.Sleep(2 * time.Second)
			continue
		}
		return db.ID, nil
	}
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(TemplatesFS, "templates/index.html")
	if err != nil {
		log.Printf("homebase index template error: %v", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	data := map[string]string{
		"BasePath": a.BasePath,
	}
	if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("homebase index render error: %v", err)
	}
}

func (a *App) RegisterRoutes(mux *http.ServeMux) {
	h := &handler{app: a}
	p := &partialHandler{app: a}

	mux.HandleFunc("GET /{$}", a.handleIndex)
	mux.HandleFunc("GET /swagger.json", HandleSwagger)
	mux.HandleFunc("GET /help.md", handleHelpMarkdown)

	// Read endpoints (no scope needed)
	mux.HandleFunc("GET /api/devices", h.handleListDevices)
	mux.HandleFunc("GET /api/devices/{id}", h.handleGetDevice)
	mux.HandleFunc("GET /api/devices/{id}/state", h.handleGetDeviceState)
	mux.HandleFunc("GET /api/devices/{id}/snapshot", h.handleGetSnapshot)
	mux.HandleFunc("GET /api/rooms", h.handleListRooms)
	mux.HandleFunc("GET /api/clusters", h.handleListClusters)
	mux.HandleFunc("GET /api/plugins", h.handleListPlugins)
	mux.HandleFunc("GET /api/sidecar/health", h.handleSidecarHealth)

	// Write endpoints (require write scope)
	mux.HandleFunc("POST /api/devices", client.RequireScopeFunc(client.ScopeWrite, h.handleCommission))
	mux.HandleFunc("PUT /api/devices/{id}", client.RequireScopeFunc(client.ScopeWrite, h.handleUpdateDevice))
	mux.HandleFunc("DELETE /api/devices/{id}", client.RequireScopeFunc(client.ScopeWrite, h.handleDecommission))
	mux.HandleFunc("POST /api/devices/{id}/command", client.RequireScopeFunc(client.ScopeWrite, h.handleCommand))
	mux.HandleFunc("POST /api/virtual-devices", client.RequireScopeFunc(client.ScopeWrite, h.handleCreateVirtual))

	// Admin endpoints (require admin scope)
	mux.HandleFunc("GET /api/plugins/{name}/credential", client.RequireScopeFunc(client.ScopeAdmin, h.handleGetPluginCredential))
	mux.HandleFunc("PUT /api/plugins/{name}/credential", client.RequireScopeFunc(client.ScopeAdmin, h.handleSetPluginCredential))
	mux.HandleFunc("DELETE /api/plugins/{name}/credential", client.RequireScopeFunc(client.ScopeAdmin, h.handleDeletePluginCredential))

	// HTMX partials (read)
	mux.HandleFunc("GET /partials/sidebar", p.handleSidebar)
	mux.HandleFunc("GET /partials/empty", p.handleEmpty)
	mux.HandleFunc("GET /partials/device/{id}", p.handleDeviceDetail)
	mux.HandleFunc("GET /partials/add-form", p.handleAddForm)
	mux.HandleFunc("GET /partials/plugin-settings", p.handlePluginSettings)
	mux.HandleFunc("GET /partials/sidecar-status", p.handleSidecarStatus)

	// HTMX partials (write)
	mux.HandleFunc("PUT /partials/device/{id}", client.RequireScopeFunc(client.ScopeWrite, p.handleUpdateDevice))
	mux.HandleFunc("DELETE /partials/device/{id}", client.RequireScopeFunc(client.ScopeWrite, p.handleDeleteDevice))
	mux.HandleFunc("POST /partials/commission", client.RequireScopeFunc(client.ScopeWrite, p.handleCommission))
	mux.HandleFunc("POST /partials/create-virtual", client.RequireScopeFunc(client.ScopeWrite, p.handleCreateVirtual))

	// HTMX partials (admin)
	mux.HandleFunc("PUT /partials/plugin-credential/{name}", client.RequireScopeFunc(client.ScopeAdmin, p.handleSetPluginCredential))
	mux.HandleFunc("DELETE /partials/plugin-credential/{name}", client.RequireScopeFunc(client.ScopeAdmin, p.handleDeletePluginCredential))
}
