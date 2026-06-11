package m3u8

import (
	"net/url"
)

type Track struct {
	Name   string
	Length float64
	URI    *url.URL
	Attrs  map[string]string
	Tags   map[string]string
}

const (
	AttrTvgID       = "tvg-id"
	AttrTvgName     = "tvg-name"
	AttrTvgLogo     = "tvg-logo"
	AttrGroupTitle  = "group-title"
	AttrRadio       = "radio"
	AttrTvgChno     = "tvg-chno"
	AttrTvgShift    = "tvg-shift"
	AttrCatchup     = "catchup"
	AttrCatchupDays = "catchup-days"
	AttrTvgRec      = "tvg-rec"
)

const (
	TagHeader   = "EXTM3U"
	TagInf      = "EXTINF"
	TagGroup    = "EXTGRP"
	TagVLCOpt   = "EXTVLCOPT"
	TagKodiProp = "KODIPROP"
)
