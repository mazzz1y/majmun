package channelgen

import (
	"bytes"
	"context"
	"majmun/internal/logging"
	"path/filepath"
	"strings"
	"text/template"
)

type probeVars struct {
	Title          string
	Show           string
	Description    string
	Category       string
	Date           string
	Season         int
	Episode        int
	VideoCodec     string
	Width          int
	Height         int
	AspectWidth    int
	PixelFormat    string
	FrameRate      string
	FieldOrder     string
	AudioCodec     string
	AudioChannels  int
	SampleRate     int
	AudioLanguages []string
}

type fileVars struct {
	Path       string
	Rel        string
	RelNoExt   string
	Name       string
	Source     string
	SourceBase string
}

type metadataVars struct {
	Probe probeVars
	File  fileVars
}

func (c *Channel) metadataVars(it Item) metadataVars {
	rel, source := relToSource(it.File, c.sources)
	base := filepath.Base(it.File)
	sourceBase := ""
	if source != "" {
		sourceBase = filepath.Base(source)
	}
	return metadataVars{
		Probe: probeVars{
			Title:          it.Title,
			Show:           it.Show,
			Description:    it.Description,
			Category:       it.Category,
			Date:           it.Date,
			Season:         it.Season,
			Episode:        it.Episode,
			VideoCodec:     it.VideoCodec,
			Width:          it.Width,
			Height:         it.Height,
			AspectWidth:    it.AspectWidth,
			PixelFormat:    it.PixelFormat,
			FrameRate:      it.FrameRate,
			FieldOrder:     it.FieldOrder,
			AudioCodec:     it.AudioCodec,
			AudioChannels:  it.AudioChannels,
			SampleRate:     it.SampleRate,
			AudioLanguages: it.AudioLanguages,
		},
		File: fileVars{
			Path:       it.File,
			Rel:        rel,
			RelNoExt:   strings.TrimSuffix(rel, filepath.Ext(rel)),
			Name:       strings.TrimSuffix(base, filepath.Ext(base)),
			Source:     source,
			SourceBase: sourceBase,
		},
	}
}

func renderField(ctx context.Context, tmpl *template.Template, vars metadataVars, raw, field string) string {
	if tmpl == nil {
		return raw
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		logging.Error(ctx, err, "failed to render metadata field, using raw value",
			"field", field, "file", vars.File.Path)
		return raw
	}
	return buf.String()
}
