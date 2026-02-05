package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	c := DefaultConfig()

	var files []string
	if !info.IsDir() {
		files = []string{path}
	} else {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := filepath.Ext(entry.Name())
			if ext != ".yaml" && ext != ".yml" {
				continue
			}
			files = append(files, filepath.Join(path, entry.Name()))
		}
	}

	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}

		dec := yaml.NewDecoder(f)
		dec.KnownFields(true)
		if err := dec.Decode(c); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err := c.Validate(); err != nil {
			return nil, err
		}
		_ = f.Close()
	}

	return c, nil
}
