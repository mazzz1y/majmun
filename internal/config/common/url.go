package common

import (
	"fmt"
	"net/url"

	"gopkg.in/yaml.v3"
)

type URL url.URL

func (ur *URL) UnmarshalYAML(value *yaml.Node) error {
	var urlStr string
	if err := value.Decode(&urlStr); err != nil {
		return err
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return err
	}
	if u.Host == "" {
		return fmt.Errorf("url must contain a host")
	}

	*ur = URL(*u)
	return nil
}

func (ur *URL) String() string {
	u := url.URL(*ur)
	return u.String()
}

func (ur *URL) ToURL() *url.URL {
	return new(url.URL(*ur))
}
