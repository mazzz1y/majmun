package m3u8

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type M3UEncoder struct {
	writer        io.Writer
	headerAttrs   map[string]string
	headerWritten bool
	closed        bool
}

func NewEncoder(w io.Writer, headerAttrs map[string]string) *M3UEncoder {
	return &M3UEncoder{
		writer:        w,
		headerAttrs:   headerAttrs,
		headerWritten: false,
		closed:        false,
	}
}

func (e *M3UEncoder) Encode(item any) error {
	if e.closed {
		return fmt.Errorf("encoder already closed")
	}

	switch v := item.(type) {
	case *Track:
		return e.encodeTrack(v)
	default:
		return fmt.Errorf("unsupported item type: %T", item)
	}
}

func (e *M3UEncoder) writeHeader() error {
	if e.headerWritten {
		return nil
	}

	header := "#" + TagHeader

	if len(e.headerAttrs) > 0 {
		attrPairs := make([]string, 0, len(e.headerAttrs))
		for k, v := range e.headerAttrs {
			attrPairs = append(attrPairs, fmt.Sprintf(`%s="%s"`, k, v))
		}

		sort.Strings(attrPairs)
		header += " " + strings.Join(attrPairs, " ")
	}

	_, err := fmt.Fprintln(e.writer, header)
	if err != nil {
		return fmt.Errorf("error writing M3U header: %w", err)
	}

	e.headerWritten = true
	return nil
}

func (e *M3UEncoder) encodeTrack(track *Track) error {
	if !e.headerWritten {
		if err := e.writeHeader(); err != nil {
			return err
		}
	}

	extinfLine := fmt.Sprintf("#EXTINF:%.6g", track.Length)

	if len(track.Attrs) > 0 {
		attrPairs := make([]string, 0, len(track.Attrs))
		for k, v := range track.Attrs {
			attrPairs = append(attrPairs, fmt.Sprintf(`%s="%s"`, k, v))
		}

		sort.Strings(attrPairs)
		extinfLine += " " + strings.Join(attrPairs, " ")
	}

	if track.Name != "" {
		extinfLine += ","
		extinfLine += track.Name
	}

	_, err := fmt.Fprintln(e.writer, extinfLine)
	if err != nil {
		return fmt.Errorf("error writing EXTINF line: %w", err)
	}

	if len(track.Tags) > 0 {
		keys := make([]string, 0, len(track.Tags))
		for k := range track.Tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			tagLine := fmt.Sprintf("#%s:%s", key, track.Tags[key])
			_, err := fmt.Fprintln(e.writer, tagLine)
			if err != nil {
				return fmt.Errorf("error writing tag: %w", err)
			}
		}
	}

	if track.URI != nil {
		_, err := fmt.Fprintln(e.writer, track.URI.String())
		if err != nil {
			return fmt.Errorf("error writing URL: %w", err)
		}
	} else {
		return fmt.Errorf("track missing URI")
	}

	return nil
}

func (e *M3UEncoder) Close() error {
	if e.closed {
		return nil
	}

	e.closed = true

	if closer, ok := e.writer.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}
