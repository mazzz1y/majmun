package common

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

var durationRegex = regexp.MustCompile(`^(\d+)([smhdwMy])$`)

type Duration time.Duration

func (t *Duration) UnmarshalYAML(value *yaml.Node) error {
	var ttlStr string
	if err := value.Decode(&ttlStr); err != nil {
		return err
	}

	if ttlStr == "0" {
		return nil
	}

	matches := durationRegex.FindStringSubmatch(ttlStr)

	if matches == nil {
		return fmt.Errorf("invalid duration format: %s", ttlStr)
	}

	val, err := strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf("invalid duration value: %s", matches[1])
	}

	unit := matches[2]

	switch unit {
	case "s":
		*t = Duration(time.Duration(val) * time.Second)
	case "m":
		*t = Duration(time.Duration(val) * time.Minute)
	case "h":
		*t = Duration(time.Duration(val) * time.Hour)
	case "d":
		*t = Duration(time.Duration(val) * 24 * time.Hour)
	case "w":
		*t = Duration(time.Duration(val) * 7 * 24 * time.Hour)
	case "M":
		*t = Duration(time.Duration(val) * 30 * 24 * time.Hour)
	case "y":
		*t = Duration(time.Duration(val) * 365 * 24 * time.Hour)
	}

	return nil
}
