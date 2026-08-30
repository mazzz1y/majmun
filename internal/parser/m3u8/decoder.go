package m3u8

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	extinfRegex            = regexp.MustCompile(`#EXTINF:(-?\d+(\.\d+)?)\s*(.*)$`)
	attrRegex              = regexp.MustCompile(`([a-zA-Z0-9_-]+)="([^"]*)"`)
	whitelistedTagPrefixes = []string{
		"#EXT-X-",
		"#EXTGRP:",
		"#EXTBYT:",
		"#EXTSIZE:",
		"#EXTBIN:",
		"#EXTVLCOPT:",
	}
)

type M3UDecoder struct {
	reader  io.ReadCloser
	scanner *bufio.Scanner
	header  bool
	done    bool
}

func NewDecoder(r io.Reader) *M3UDecoder {
	var readCloser io.ReadCloser
	if rc, ok := r.(io.ReadCloser); ok {
		readCloser = rc
	} else {
		readCloser = io.NopCloser(r)
	}

	return &M3UDecoder{
		reader:  readCloser,
		scanner: bufio.NewScanner(r),
		header:  false,
		done:    false,
	}
}

func (d *M3UDecoder) Decode() (any, error) {
	if d.done {
		return nil, io.EOF
	}

	if !d.header {
		if !d.scanner.Scan() {
			if err := d.scanner.Err(); err != nil {
				return nil, err
			}
			return nil, io.EOF
		}

		line := strings.TrimSpace(d.scanner.Text())
		if !strings.HasPrefix(line, "#EXTM3U") {
			return nil, fmt.Errorf("invalid M3U file format, missing #EXTM3U header")
		}

		d.header = true
	}

	track, err := d.parseNextTrack()
	if err != nil {
		if errors.Is(err, io.EOF) {
			d.done = true
			d.drainReader()
		}
		return nil, err
	}

	return track, nil
}

func (d *M3UDecoder) parseNextTrack() (*Track, error) {
	var track *Track

	for d.scanner.Scan() {
		line := strings.TrimSpace(d.scanner.Text())

		if line == "" || (strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#EXT")) {
			continue
		}

		if strings.HasPrefix(line, "#EXTINF:") {
			var err error
			track, err = parseExtInfLine(line)
			if err != nil {
				return nil, fmt.Errorf("error parsing EXTINF line: %w", err)
			}
			continue
		}

		if track != nil && hasWhitelistedTagPrefix(line) {
			maps.Copy(track.Tags, parseTags(line))
			continue
		}

		if track != nil && !strings.HasPrefix(line, "#") {
			u, err := url.Parse(line)
			if err != nil {
				return nil, fmt.Errorf("invalid URL: %w", err)
			}
			track.URI = u
			return track, nil
		}
	}

	if err := d.scanner.Err(); err != nil {
		return nil, err
	}

	return nil, io.EOF
}

func (d *M3UDecoder) Close() error {
	return d.reader.Close()
}

func parseExtInfLine(line string) (*Track, error) {
	matches := extinfRegex.FindStringSubmatch(line)
	if len(matches) < 3 {
		return nil, fmt.Errorf("invalid EXTINF format")
	}

	duration, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return nil, err
	}

	track := &Track{
		Length: duration,
		Attrs:  make(map[string]string),
		Tags:   make(map[string]string),
	}

	remainingContent := matches[3]

	attrMatches := attrRegex.FindAllStringSubmatch(remainingContent, -1)
	for _, match := range attrMatches {
		if len(match) > 2 {
			track.Attrs[match[1]] = match[2]
		}
	}

	_, title, found := strings.CutLast(remainingContent, ",")
	if found {
		track.Name = strings.TrimSpace(title)
	} else if len(attrMatches) == 0 {
		track.Name = strings.TrimSpace(remainingContent)
	}

	return track, nil
}

func hasWhitelistedTagPrefix(line string) bool {
	for _, prefix := range whitelistedTagPrefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func parseTags(line string) map[string]string {
	t := make(map[string]string)

	for _, prefix := range whitelistedTagPrefixes {
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		var key, value string
		if prefix == "#EXT-X-" {
			if before, after, found := strings.Cut(line, ":"); found {
				key = strings.TrimPrefix(before, "#")
				value = after
			}
		} else {
			key = strings.TrimSuffix(strings.TrimPrefix(prefix, "#"), ":")
			value = strings.TrimPrefix(line, prefix)
		}

		if key != "" {
			t[key] = value
			return t
		}
	}

	return t
}

func (d *M3UDecoder) drainReader() {
	if d.reader == nil {
		return
	}

	buf := make([]byte, 64)
	for {
		_, err := d.reader.Read(buf)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break
		}
	}
}
