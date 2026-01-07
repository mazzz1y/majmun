package httpclient

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

var forwardedHeaders = []string{
	"Cache-Control", "Expires", "Last-Modified", "ETag", "Content-Type",
}

func (r *Reader) SaveMetadata() error {
	if r.TmpMetaPath == "" {
		r.TmpMetaPath = r.MetaPath + ".tmp"
	}
	_ = os.Remove(r.TmpMetaPath)
	metaFile, err := os.Create(r.TmpMetaPath)
	if err != nil {
		return err
	}
	defer func() { _ = metaFile.Close() }()

	headers := make(map[string]string, len(forwardedHeaders))

	if r.originResponse != nil {
		for _, header := range forwardedHeaders {
			if value := r.originResponse.Header.Get(header); value != "" {
				headers[header] = value
			}
		}
	}
	ret := int64(r.retention / time.Second)

	if err := json.NewEncoder(metaFile).Encode(Metadata{
		CachedAt:         time.Now().Unix(),
		RetentionSeconds: &ret,
		Headers:          headers,
	}); err != nil {
		return err
	}
	if err := metaFile.Close(); err != nil {
		return err
	}
	_ = os.Remove(r.MetaPath)
	return os.Rename(r.TmpMetaPath, r.MetaPath)
}

func (r *Reader) Cleanup() {
	_ = os.Remove(r.FilePath)
	_ = os.Remove(r.TmpFilePath)
	_ = os.Remove(r.MetaPath)
	_ = os.Remove(r.TmpMetaPath)
}

func readMetadata(metaPath string) (Metadata, error) {
	metaFile, err := os.Open(metaPath)
	if err != nil {
		return Metadata{}, err
	}
	defer func() { _ = metaFile.Close() }()

	var m Metadata
	if err := json.NewDecoder(metaFile).Decode(&m); err != nil {
		return Metadata{}, fmt.Errorf("invalid meta file format: %w", err)
	}

	return m, nil
}
