package server

import (
	"errors"
	"majmun/internal/app"
	"majmun/internal/ctxutil"
	"majmun/internal/logging"
	"majmun/internal/urlgen"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func (s *Server) loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)
		duration := time.Since(startTime)
		logging.HttpRequest(r.Context(), r, rw.statusCode, duration, rw.bytesWritten)
	})
}

func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := ctxutil.WithRequestID(r.Context())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) clientAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		secret, ok := vars[muxClientSecretVar]
		if !ok || secret == "" {
			logging.Debug(r.Context(), "authentication failed: no secret")
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		client := s.manager.Client(secret)
		if client == nil {
			logging.Debug(r.Context(), "authentication failed: invalid secret", "secret", secret)
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		ctx := ctxutil.WithClient(r.Context(), client)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) proxyAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := mux.Vars(r)[muxEncryptedTokenVar]
		if !ok || token == "" {
			logging.Debug(r.Context(), "authentication failed: no token")
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		ctx := r.Context()

		for _, client := range s.manager.Clients() {
			data, err := client.URLGenerator().Decrypt(token)

			if err == nil && data != nil {
				provider := s.getProviderFromData(client, data)
				if provider == nil {
					continue
				}

				ctx = ctxutil.WithClient(ctx, client)
				ctx = ctxutil.WithProvider(ctx, provider)
				ctx = ctxutil.WithStreamData(ctx, data)
				ctx = ctxutil.WithProviderType(ctx, provider.Type())
				ctx = ctxutil.WithProviderName(ctx, provider.Name())
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if errors.Is(err, urlgen.ErrExpiredStreamURL) {
				provider := s.getProviderFromData(client, data)
				if provider != nil && provider.ExpiredLinkStreamer() != nil {
					ctx = ctxutil.WithClient(ctx, client)
					_, err := provider.ExpiredLinkStreamer().Stream(ctx, w)
					if err != nil {
						logging.Error(ctx, err, "failed to stream expired link response")
					}
				} else {
					http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				}
				return
			}
		}

		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	})
}

func (s *Server) getProviderFromData(client *app.Client, data *urlgen.Data) app.Provider {
	if data == nil {
		return nil
	}

	var providerInfo *urlgen.ProviderInfo

	switch data.RequestType {
	case urlgen.RequestTypeStream:
		if len(data.StreamData.Streams) > 0 {
			providerInfo = &data.StreamData.Streams[0].ProviderInfo
		}
	case urlgen.RequestTypeFile:
		providerInfo = &data.File.ProviderInfo
	}

	if providerInfo == nil {
		return nil
	}

	return client.GetProvider(providerInfo.ProviderType, providerInfo.ProviderName)
}
