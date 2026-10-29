// Package controllers implements the admin web server: route registration,
// middleware wiring, and HTML template rendering for the Phish & Discover dashboard.
//
// Handlers are split across three files by responsibility:
//   - route.go: server bootstrap, route registration, and template plumbing
//   - route_pages.go: simple dashboard pages that just render a template
//   - route_auth.go: login, logout, password reset, and impersonation
package controllers

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"html/template"
	"net/http"
	"time"

	csrf "filippo.io/csrf/gorilla"
	"github.com/NYTimes/gziphandler"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/jordan-wright/unindexed"
	"github.com/ytalharabaci/phish-and-discover/auth"
	"github.com/ytalharabaci/phish-and-discover/config"
	ctx "github.com/ytalharabaci/phish-and-discover/context"
	"github.com/ytalharabaci/phish-and-discover/controllers/api"
	log "github.com/ytalharabaci/phish-and-discover/logger"
	mid "github.com/ytalharabaci/phish-and-discover/middleware"
	"github.com/ytalharabaci/phish-and-discover/middleware/ratelimit"
	"github.com/ytalharabaci/phish-and-discover/models"
	"github.com/ytalharabaci/phish-and-discover/scanner"
	"github.com/ytalharabaci/phish-and-discover/util"
	"github.com/ytalharabaci/phish-and-discover/worker"
)

// AdminServerOption is a functional option that is used to configure the
// admin server
type AdminServerOption func(*AdminServer)

// AdminServer is an HTTP server that implements the administrative Gophish
// handlers, including the dashboard and REST API.
type AdminServer struct {
	server  *http.Server
	worker  worker.Worker
	config  config.AdminServer
	limiter *ratelimit.PostLimiter
}

var defaultTLSConfig = &tls.Config{
	PreferServerCipherSuites: true,
	CurvePreferences: []tls.CurveID{
		tls.X25519,
		tls.CurveP256,
	},
	MinVersion: tls.VersionTLS12,
	CipherSuites: []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,

		// Kept for backwards compatibility with some clients
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	},
}

// WithWorker is an option that sets the background worker.
func WithWorker(w worker.Worker) AdminServerOption {
	return func(as *AdminServer) {
		as.worker = w
	}
}

// NewAdminServer returns a new instance of the AdminServer with the
// provided config and options applied.
func NewAdminServer(config config.AdminServer, options ...AdminServerOption) *AdminServer {
	defaultWorker, _ := worker.New()
	defaultServer := &http.Server{
		ReadTimeout: 10 * time.Second,
		Addr:        config.ListenURL,
	}
	defaultLimiter := ratelimit.NewPostLimiter()
	as := &AdminServer{
		worker:  defaultWorker,
		server:  defaultServer,
		limiter: defaultLimiter,
		config:  config,
	}
	for _, opt := range options {
		opt(as)
	}
	as.registerRoutes()
	return as
}

// Start launches the admin server, listening on the configured address.
func (as *AdminServer) Start() {
	if as.worker != nil {
		go as.worker.Start()
	}
	if as.config.UseTLS {
		// Only support TLS 1.2 and above - ref #1691, #1689
		as.server.TLSConfig = defaultTLSConfig
		err := util.CheckAndCreateSSL(as.config.CertPath, as.config.KeyPath)
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Starting admin server at https://%s", as.config.ListenURL)
		log.Fatal(as.server.ListenAndServeTLS(as.config.CertPath, as.config.KeyPath))
	}
	// If TLS isn't configured, just listen on HTTP
	log.Infof("Starting admin server at http://%s", as.config.ListenURL)
	log.Fatal(as.server.ListenAndServe())
}

// Shutdown attempts to gracefully shutdown the server.
func (as *AdminServer) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	return as.server.Shutdown(ctx)
}

// SetupAdminRoutes creates the routes for handling requests to the web interface.
// This function returns an http.Handler to be used in http.ListenAndServe().
func (as *AdminServer) registerRoutes() {
	router := mux.NewRouter()
	// Base Front-end routes
	router.HandleFunc("/", mid.Use(as.Base, mid.RequireLogin))
	router.HandleFunc("/login", mid.Use(as.Login, as.limiter.Limit))
	router.HandleFunc("/logout", mid.Use(as.Logout, mid.RequireLogin))
	router.HandleFunc("/reset_password", mid.Use(as.ResetPassword, mid.RequireLogin))
	router.HandleFunc("/campaigns", mid.Use(as.Campaigns, mid.RequireLogin))
	router.HandleFunc("/campaigns/{id:[0-9]+}", mid.Use(as.CampaignID, mid.RequireLogin))
	router.HandleFunc("/templates", mid.Use(as.Templates, mid.RequireLogin))
	router.HandleFunc("/groups", mid.Use(as.Groups, mid.RequireLogin))
	router.HandleFunc("/landing_pages", mid.Use(as.LandingPages, mid.RequireLogin))
	router.HandleFunc("/sending_profiles", mid.Use(as.SendingProfiles, mid.RequireLogin))
	router.HandleFunc("/settings", mid.Use(as.Settings, mid.RequireLogin))
	router.HandleFunc("/users", mid.Use(as.UserManagement, mid.RequirePermission(models.PermissionModifySystem), mid.RequireLogin))
	router.HandleFunc("/webhooks", mid.Use(as.Webhooks, mid.RequirePermission(models.PermissionModifySystem), mid.RequireLogin))
	router.HandleFunc("/impersonate", mid.Use(as.Impersonate, mid.RequirePermission(models.PermissionModifySystem), mid.RequireLogin))

	// ── FINAL SCANNER ROUTER ───────────────────────────────────────────────────────
	// Discover security scanning subsystem routes
	router.HandleFunc("/ws/scanner/logs", mid.Use(scanner.ScannerWSHandler, mid.RequireLogin))
	// Create the API routes
	api := api.NewServer(
		api.WithWorker(as.worker),
		api.WithLimiter(as.limiter),
	)
	router.PathPrefix("/api/").Handler(api)

	// Setup static file serving
	router.PathPrefix("/").Handler(http.FileServer(unindexed.Dir("./static/")))

	// Setup CSRF Protection
	csrfKey := []byte(as.config.CSRFKey)
	if len(csrfKey) == 0 {
		csrfKey = []byte(auth.GenerateSecureKey(auth.APIKeyLength))
	}
	csrfHandler := csrf.Protect(csrfKey,
		csrf.FieldName("csrf_token"),
		csrf.Secure(as.config.UseTLS),
		csrf.TrustedOrigins(as.config.TrustedOrigins))
	adminHandler := csrfHandler(router)
	if !as.config.UseTLS {
		// We use filippo.io/csrf/gorilla, a drop-in replacement for
		// github.com/gorilla/csrf, since the latter has an unpatched CVE in
		// its TrustedOrigins handling (origins are matched on host only,
		// implicitly trusting both their HTTP and HTTPS forms). The
		// replacement enforces same-origin checks via Fetch metadata
		// instead of tokens, but keeps this package's API, including
		// assuming HTTPS unless the request is explicitly marked as
		// plaintext. Mirror the Secure(as.config.UseTLS) option above so
		// non-TLS deployments don't have every unsafe request rejected for
		// a missing/HTTPS-only Referer.
		next := adminHandler
		adminHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, csrf.PlaintextHTTPRequest(r))
		})
	}
	adminHandler = mid.Use(adminHandler.ServeHTTP, mid.CSRFExceptions, mid.GetContext, mid.ApplySecurityHeaders)

	// Setup GZIP compression
	gzipWrapper, _ := gziphandler.NewGzipLevelHandler(gzip.BestCompression)
	adminHandler = gzipWrapper(adminHandler)

	// Respect X-Forwarded-For and X-Real-IP headers in case we're behind a
	// reverse proxy.
	adminHandler = handlers.ProxyHeaders(adminHandler)

	// Setup logging
	adminHandler = handlers.CombinedLoggingHandler(log.Writer(), adminHandler)
	as.server.Handler = adminHandler
}

type templateParams struct {
	Title             string
	Flashes           []interface{}
	User              models.User
	Token             string
	Version           string
	ModifySystem      bool
	PDTools           []string
	ChiselFingerprint string
	Host              string
}

// newTemplateParams returns the default template parameters for a user and
// the CSRF token.
func newTemplateParams(r *http.Request) templateParams {
	user := ctx.Get(r, "user").(models.User)
	session := ctx.Get(r, "session").(*sessions.Session)
	modifySystem, _ := user.HasPermission(models.PermissionModifySystem)
	params := templateParams{
		Token:        csrf.Token(r),
		User:         user,
		ModifySystem: modifySystem,
		Version:      config.Version,
		Flashes:      session.Flashes(),
		PDTools: []string{
			"subfinder", "httpx", "nuclei", "naabu",
			"dnsx", "katana", "tlsx", "uncover",
			"asnmap", "interactsh-client", "assetfinder", "cloudlist",
		},
		ChiselFingerprint: "SHA256:PhishDiscoverTacticalSectorAccessKeyFingerprint",
		Host:              r.Host,
	}
	return params
}

// TODO: Make this execute the template, too
func getTemplate(w http.ResponseWriter, tmpl string) *template.Template {
	templates := template.New("template")
	// First parse the specific templates needed for the page
	_, err := templates.ParseFiles("templates/base.html", "templates/nav.html", "templates/"+tmpl+".html", "templates/flashes.html")
	if err != nil {
		log.Error(err)
	}
	// Also parse all component templates in discover subfolder.
	// Use a separate error variable so a glob failure does not
	// overwrite `err` and cause template.Must to panic when the
	// primary templates parsed successfully.
	_, globErr := templates.ParseGlob("templates/discover/*.html")
	if globErr != nil {
		log.Warnf("Template parsing warning (discover/): %v", globErr)
	}
	return template.Must(templates, err)
}

// Flash handles the rendering flash messages
func Flash(w http.ResponseWriter, r *http.Request, t string, m string) {
	session := ctx.Get(r, "session").(*sessions.Session)
	session.AddFlash(models.Flash{
		Type:    t,
		Message: m,
	})
}
