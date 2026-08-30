package channelgen

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type scannedFile struct {
	path  string
	size  int64
	mtime int64
}

func scanSources(sources []string, extensions []string) ([]scannedFile, error) {
	allowed := make(map[string]bool, len(extensions))
	for _, ext := range extensions {
		allowed[normalizeExt(ext)] = true
	}

	var files []scannedFile
	seen := make(map[string]bool)

	add := func(path string, info fs.FileInfo) {
		if info.IsDir() || !allowed[normalizeExt(filepath.Ext(path))] {
			return
		}
		if seen[path] {
			return
		}
		seen[path] = true
		files = append(files, scannedFile{
			path:  path,
			size:  info.Size(),
			mtime: info.ModTime().Unix(),
		})
	}

	for _, source := range sources {
		info, err := os.Stat(source)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(source, info)
			continue
		}
		err = filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			add(path, fi)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	slices.SortFunc(files, func(a, b scannedFile) int {
		return cmp.Compare(a.path, b.path)
	})
	return files, nil
}

func fingerprint(files, fillerFiles []scannedFile, order Order, seasonPatterns, episodePatterns []*regexp.Regexp, filler FillerConfig) string {
	h := sha256.New()
	_, _ = h.Write([]byte("order:" + string(order) + "\x00"))
	writePatterns(h, "season", seasonPatterns)
	writePatterns(h, "episode", episodePatterns)
	writeFiles(h, files)
	_, _ = fmt.Fprintf(h, "filler:%d:%d:%d:%s\x00", filler.EveryCount, filler.Every, filler.MaxDuration, filler.Order)
	writeFiles(h, fillerFiles)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func writePatterns(h hash.Hash, label string, patterns []*regexp.Regexp) {
	for _, re := range patterns {
		_, _ = fmt.Fprintf(h, "%s:%s\x00", label, re.String())
	}
}

func writeFiles(h hash.Hash, files []scannedFile) {
	for _, f := range files {
		_, _ = h.Write([]byte(f.path))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(strconv.FormatInt(f.size, 10)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(strconv.FormatInt(f.mtime, 10)))
		_, _ = h.Write([]byte{0})
	}
}

func normalizeExt(ext string) string {
	return strings.ToLower(strings.TrimPrefix(ext, "."))
}

func titleFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
