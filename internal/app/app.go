package app

import (
	"context"
	"fmt"
	"majmun/internal/config"
	"majmun/internal/httpclient"
	"majmun/internal/logging"
	"majmun/internal/metrics"
	"majmun/internal/urlgen"
	"time"

	"golang.org/x/sync/semaphore"
)

type Manager struct {
	config         *config.Config
	semaphore      *semaphore.Weighted
	clients        []*Client
	secretToClient map[string]*Client
	publicURLBase  string
	cacheStore     *httpclient.Store
}

func NewManager(cfg *config.Config) (*Manager, error) {
	m := &Manager{
		config:         cfg,
		secretToClient: make(map[string]*Client),
		publicURLBase:  cfg.Server.PublicURL.String(),
	}

	if cfg.Proxy.Enabled != nil && *cfg.Proxy.Enabled && cfg.Proxy.ConcurrentStreams > 0 {
		m.semaphore = semaphore.NewWeighted(cfg.Proxy.ConcurrentStreams)
	}

	if cfg.HTTPClient.Cache.Enabled != nil && *cfg.HTTPClient.Cache.Enabled {
		if cfg.HTTPClient.Cache.Path == nil {
			return nil, fmt.Errorf("http_client.cache.path is required when cache is enabled")
		}
		st, err := httpclient.NewStore(*cfg.HTTPClient.Cache.Path)
		if err != nil {
			return nil, err
		}
		m.cacheStore = st
	}

	if err := m.initClients(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manager) Client(secret string) *Client {
	return m.secretToClient[secret]
}

func (m *Manager) Clients() []*Client {
	return m.clients
}

func (m *Manager) Semaphore() *semaphore.Weighted {
	return m.semaphore
}

func (m *Manager) initClients() error {
	m.clients = make([]*Client, 0, len(m.config.Clients))

	for _, clientConf := range m.config.Clients {
		clientInstance, err := m.createClient(clientConf)
		if err != nil {
			return err
		}

		m.clients = append(m.clients, clientInstance)
		m.secretToClient[clientConf.Secret] = clientInstance

		logging.Debug(context.TODO(), "client initialized", "name", clientConf.Name)
	}

	return nil
}

func (m *Manager) createClient(clientConf config.Client) (*Client, error) {
	urlGen, err := m.createURLGenerator(clientConf.Secret)
	if err != nil {
		return nil, fmt.Errorf("failed to create URL generator: %w", err)
	}

	cl, err := NewClient(clientConf, urlGen, m.config.ChannelRules, m.config.PlaylistRules, m.publicURLBase, m.cacheStore, m.config.HTTPClient)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to initialize client %s: %w", clientConf.Name, err)
	}

	if err := m.initClientProviders(cl, clientConf.Playlists, clientConf.EPGs); err != nil {
		return nil, fmt.Errorf(
			"failed to add subscriptions for client %s: %w", clientConf.Name, err)
	}

	return cl, nil
}

func (m *Manager) initClientProviders(cl *Client, playlistNames, epgNames []string) error {
	if len(playlistNames) == 0 {
		for _, playlist := range m.config.Playlists {
			if err := m.addPlaylistProvider(cl, playlist); err != nil {
				return err
			}
		}
	} else {
		for _, playlistName := range playlistNames {
			playlistConf, err := m.findPlaylist(playlistName)
			if err != nil {
				return fmt.Errorf("playlist '%s' for client '%s' is not defined in config", playlistName, cl.name)
			}
			if err := m.addPlaylistProvider(cl, playlistConf); err != nil {
				return err
			}
		}
	}

	if len(epgNames) == 0 {
		for _, epg := range m.config.EPGs {
			if err := m.addEPGProvider(cl, epg); err != nil {
				return err
			}
		}
	} else {
		for _, epgName := range epgNames {
			epgConf, err := m.findEPG(epgName)
			if err != nil {
				return fmt.Errorf("EPG '%s' for client '%s' is not defined in config", epgName, cl.name)
			}
			if err := m.addEPGProvider(cl, epgConf); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *Manager) addPlaylistProvider(cl *Client, playlistConf config.Playlist) error {
	var sem *semaphore.Weighted
	if playlistConf.Proxy.ConcurrentStreams > 0 {
		sem = semaphore.NewWeighted(playlistConf.Proxy.ConcurrentStreams)
	}

	metrics.SetPlaylistStreamsActive(playlistConf.Name, 0)

	if err := cl.BuildPlaylistProvider(
		playlistConf, m.config.Proxy, sem); err != nil {
		return fmt.Errorf(
			"failed to build playlist subscription '%s' for client '%s': %w",
			playlistConf.Name, cl.name, err)
	}

	return nil
}

func (m *Manager) addEPGProvider(cl *Client, epgConf config.EPG) error {
	if err := cl.BuildEPGProvider(epgConf, m.config.Proxy); err != nil {
		return fmt.Errorf(
			"failed to build EPG subscription '%s' for client '%s': %w",
			epgConf.Name, cl.name, err)
	}
	return nil
}

func (m *Manager) findPlaylist(name string) (config.Playlist, error) {
	for _, playlist := range m.config.Playlists {
		if playlist.Name == name {
			return playlist, nil
		}
	}
	return config.Playlist{}, fmt.Errorf("playlist not found: %s", name)
}

func (m *Manager) findEPG(name string) (config.EPG, error) {
	for _, epg := range m.config.EPGs {
		if epg.Name == name {
			return epg, nil
		}
	}
	return config.EPG{}, fmt.Errorf("EPG not found: %s", name)
}

func (m *Manager) createURLGenerator(clientSecret string) (*urlgen.Generator, error) {
	secretKey := m.config.URLGenerator.Secret + clientSecret

	return urlgen.NewGenerator(
		m.publicURLBase, secretKey,
		time.Duration(m.config.URLGenerator.StreamTTL),
		time.Duration(m.config.URLGenerator.FileTTL),
	)
}
