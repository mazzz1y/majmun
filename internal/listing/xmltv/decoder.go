package xmltv

import (
	"context"
	"io"
	"majmun/internal/listing"
	"majmun/internal/parser/xmltv"
)

type decoderWrapper struct {
	*listing.BaseDecoder
	subscription listing.EPG
	sourceURL    string
}

func newDecoderWrapper(subscription listing.EPG, httpClient listing.HTTPClient, url string) *decoderWrapper {
	initializer := func(ctx context.Context, url string) (listing.Decoder, io.ReadCloser, error) {
		reader, err := listing.CreateReader(ctx, httpClient, url)
		if err != nil {
			return nil, nil, err
		}
		decoder := xmltv.NewDecoder(reader)
		return decoder, reader, nil
	}

	return &decoderWrapper{
		BaseDecoder:  listing.NewLazyBaseDecoder(url, initializer),
		subscription: subscription,
		sourceURL:    url,
	}
}
