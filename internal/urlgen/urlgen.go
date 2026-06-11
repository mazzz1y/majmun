package urlgen

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

type RequestType int
type ProviderType int

const (
	RequestTypeStream RequestType = iota
	RequestTypeFile
)

const (
	ProviderTypePlaylist ProviderType = iota
	ProviderTypeEPG
	ProviderTypeChannel
)

var (
	ErrExpiredStreamURL = errors.New("expired stream URL")
	ErrExpiredFileURL   = errors.New("expired file URL")
	ErrInvalidToken     = errors.New("invalid token")
)

type Generator struct {
	PublicURL string
	aead      cipher.AEAD
	streamTTL time.Duration
	fileTTL   time.Duration
}

type Data struct {
	RequestType RequestType `json:"rt"`
	CreatedAt   int64       `json:"ca"`
	StreamData  StreamData  `json:"sd,omitempty"`
	File        FileData    `json:"f,omitempty"`
}

type ProviderInfo struct {
	ProviderType ProviderType `json:"pt"`
	ProviderID   string       `json:"pid"`
}

type StreamData struct {
	ChannelName string   `json:"cn"`
	Streams     []Stream `json:"s"`
}

type Stream struct {
	ProviderInfo ProviderInfo `json:"pi"`
	URL          string       `json:"u"`
	Hidden       bool         `json:"h,omitempty"`
}

type FileData struct {
	ProviderInfo ProviderInfo `json:"pi"`
	URL          string       `json:"u"`
}

func NewGenerator(publicURL, secret string, streamTTL, fileTTL time.Duration) (*Generator, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	return &Generator{PublicURL: publicURL, aead: aead, streamTTL: streamTTL, fileTTL: fileTTL}, nil
}

func (g *Generator) CreateStreamURL(channelName string, streams []Stream) (*url.URL, error) {
	return g.CreateURL(Data{
		RequestType: RequestTypeStream,
		StreamData:  StreamData{ChannelName: channelName, Streams: streams},
		CreatedAt:   time.Now().Unix(),
	})
}

func (g *Generator) CreateFileURL(providerInfo ProviderInfo, fileURL string) (*url.URL, error) {
	return g.CreateURL(Data{
		RequestType: RequestTypeFile,
		File: FileData{
			ProviderInfo: providerInfo,
			URL:          fileURL,
		},
		CreatedAt: time.Now().Unix(),
	})
}

func (g *Generator) CreateURL(d Data) (*url.URL, error) {
	jsonData, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	hash := sha256.Sum256(jsonData)
	nonce := hash[:g.aead.NonceSize()]

	ct := g.aead.Seal(nonce, nonce, jsonData, nil)
	token := base64.RawURLEncoding.EncodeToString(ct)
	ext := determineExtension(d)

	u, err := url.Parse(fmt.Sprintf("%s/%s/f%s", g.PublicURL, token, ext))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}
	return u, nil
}

func (g *Generator) Decrypt(token string) (*Data, error) {
	ct, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("failed to decode token: %w", err)
	}

	nonceSize := g.aead.NonceSize()
	if len(ct) < nonceSize {
		return nil, ErrInvalidToken
	}

	plain, err := g.aead.Open(nil, ct[:nonceSize], ct[nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	var data Data
	if err := json.Unmarshal(plain, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	if data.CreatedAt > 0 {
		createdTime := time.Unix(data.CreatedAt, 0)
		var ttl time.Duration
		if data.RequestType == RequestTypeStream {
			ttl = g.streamTTL
		} else {
			ttl = g.fileTTL
		}

		if ttl > 0 && createdTime.Add(ttl).Before(time.Now()) {
			if data.RequestType == RequestTypeStream {
				return nil, ErrExpiredStreamURL
			}
			return nil, ErrExpiredFileURL
		}
	}

	return &data, nil
}

func determineExtension(d Data) string {
	if d.RequestType == RequestTypeStream {
		return ".ts"
	}

	if d.File.URL == "" {
		return ".file"
	}

	base, _, _ := strings.Cut(d.File.URL, "?")
	ext := filepath.Ext(base)
	if ext == "" {
		return ".file"
	}
	return ext
}
