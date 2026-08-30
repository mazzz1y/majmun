package xmltv

import (
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

const xmlDecoderBufferSize = 64 * 1024

type XMLDecoder struct {
	reader  io.Reader
	decoder *xml.Decoder
	done    bool
}

func NewDecoder(r io.Reader) *XMLDecoder {
	bufferedReader := bufio.NewReaderSize(r, xmlDecoderBufferSize)

	return &XMLDecoder{
		reader:  bufferedReader,
		decoder: xml.NewDecoder(bufferedReader),
		done:    false,
	}
}

func (d *XMLDecoder) Decode() (any, error) {
	if d.done {
		return nil, io.EOF
	}

	for {
		tok, err := d.decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				d.done = true
			}
			return nil, err
		}

		if se, ok := tok.(xml.StartElement); ok {
			switch se.Name.Local {
			case "tv":
				tv := TV{}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "date":
						tv.Date = attr.Value
					case "source-info-url":
						tv.SourceInfoURL = attr.Value
					case "source-info-name":
						tv.SourceInfoName = attr.Value
					case "source-data-url":
						tv.SourceDataURL = attr.Value
					case "generator-info-name":
						tv.GeneratorInfoName = attr.Value
					case "generator-info-url":
						tv.GeneratorInfoURL = attr.Value
					}
				}
				return tv, nil

			case "channel":
				var channel Channel
				if err := d.decoder.DecodeElement(&channel, &se); err != nil {
					return nil, fmt.Errorf("error decoding channel: %w", err)
				}
				return channel, nil

			case "programme":
				var programme Programme
				if err := d.decoder.DecodeElement(&programme, &se); err != nil {
					return nil, fmt.Errorf("error decoding programme: %w", err)
				}
				return programme, nil
			}
		} else if ee, ok := tok.(xml.EndElement); ok && ee.Name.Local == "tv" {
			d.done = true
			return nil, io.EOF
		}
	}
}
