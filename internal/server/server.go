package server

import (
	"context"
	"errors"
	"majmun/internal/app"
	"majmun/internal/config"
	"majmun/internal/demux"
	"majmun/internal/logging"
	"majmun/internal/metrics"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	muxClientSecretVar   = "client_secret"
	muxEncryptedTokenVar = "encrypted_token"
)

type Server struct {
	router  *mux.Router
	server  *http.Server
	manager *app.Manager

	demux *demux.Demuxer

	serverURL     string
	listenAddr    string
	metricsServer *http.Server

	ctx    context.Context
	cancel context.CancelFunc
}

func NewServer(cfg *config.Config) (*Server, error) {
	m, err := app.NewManager(cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	server := &Server{
		router:     mux.NewRouter(),
		manager:    m,
		demux:      demux.NewDemuxer(),
		serverURL:  cfg.Server.PublicURL.String(),
		listenAddr: cfg.Server.ListenAddr,
		ctx:        ctx,
		cancel:     cancel,
	}

	if cfg.Server.MetricsAddr != "" {
		server.setupMetricsServer(cfg.Server.MetricsAddr)
	}

	return server, nil
}

func (s *Server) Start() error {
	s.setupRoutes()

	if s.metricsServer != nil {
		go func() {
			logging.Info(s.ctx, "starting metrics server", "address", s.metricsServer.Addr)
			if err := s.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logging.Error(s.ctx, err, "metrics server failed")
			}
		}()
	}

	s.server = &http.Server{
		Addr:    s.listenAddr,
		Handler: s.router,
	}

	logging.Info(s.ctx, "starting http server", "address", s.listenAddr)

	err := s.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logging.Error(s.ctx, err, "server failed")
		return err
	}

	return nil
}

func (s *Server) Stop() error {
	s.cancel()

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	s.demux.Stop()

	logging.Info(ctx, "stopping http server")
	if err := s.server.Shutdown(ctx); err != nil {
		logging.Error(ctx, err, "server shutdown timeout, force closing connections")
		_ = s.server.Close()
	}

	if s.metricsServer != nil {
		logging.Info(ctx, "stopping metrics server")
		if err := s.metricsServer.Shutdown(ctx); err != nil {
			logging.Error(ctx, err, "metrics server shutdown timeout, force closing connections")
			_ = s.metricsServer.Close()
		}
	}

	logging.Info(ctx, "server stopped")
	return nil
}

func (s *Server) setupRoutes() {
	s.router.HandleFunc("/healthz", s.handleHealthz)

	if s.metricsServer != nil {
		s.router.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))
	}

	s.router.Use(s.requestIDMiddleware)
	s.router.Use(s.loggerMiddleware)

	clientRouter := s.router.PathPrefix("/{" + muxClientSecretVar + "}").Subrouter()
	clientRouter.Use(s.clientAuthMiddleware)
	clientRouter.HandleFunc("/playlist.m3u8", s.handlePlaylist)
	clientRouter.HandleFunc("/epg.xml", s.handleEPG)
	clientRouter.HandleFunc("/epg.xml.gz", s.handleEPGgz)

	proxyRouter := s.router.PathPrefix("/{" + muxEncryptedTokenVar + "}").Subrouter()
	proxyRouter.Use(s.proxyAuthMiddleware)
	proxyRouter.HandleFunc("/{.*}", s.handleProxy)
}

func (s *Server) setupMetricsServer(addr string) {
	m := http.NewServeMux()
	m.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))

	s.metricsServer = &http.Server{
		Addr:    addr,
		Handler: m,
	}
}
