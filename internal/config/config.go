package config

import (
	"fmt"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	"majmun/internal/config/rules/channel"
	"majmun/internal/config/rules/playlist"
	"strings"
)

type Config struct {
	YamlSnippets  map[string]any     `yaml:",inline"`
	Server        ServerConfig       `yaml:"server"`
	Logs          Logs               `yaml:"logs"`
	URLGenerator  URLGeneratorConfig `yaml:"url_generator"`
	Proxy         proxy.Proxy        `yaml:"proxy"`
	Clients       []Client           `yaml:"clients"`
	Playlists     []Playlist         `yaml:"playlists"`
	EPGs          []EPG              `yaml:"epgs"`
	ChannelRules  channel.Rules      `yaml:"channel_rules,omitempty"`
	PlaylistRules playlist.Rules     `yaml:"playlist_rules,omitempty"`
}

func (c *Config) Validate() error {
	for key := range c.YamlSnippets {
		if !strings.HasPrefix(key, ".") {
			return fmt.Errorf("unknown config key: %s", key)
		}
	}

	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server configuration validation failed: %w", err)
	}

	if err := c.Logs.Validate(); err != nil {
		return fmt.Errorf("logs configuration validation failed: %w", err)
	}

	if err := c.URLGenerator.Validate(); err != nil {
		return fmt.Errorf("url_generator configuration validation failed: %w", err)
	}

	if err := c.Proxy.ValidateGlobal(); err != nil {
		return fmt.Errorf("proxy configuration validation failed: %w", err)
	}

	playlistNames := make(map[string]bool)
	epgNames := make(map[string]bool)

	for i, pl := range c.Playlists {
		if err := pl.Validate(); err != nil {
			return fmt.Errorf("playlist[%d] validation failed: %w", i, err)
		}
		if pl.Name != "" {
			if playlistNames[pl.Name] {
				return fmt.Errorf("duplicate playlist name: %s", pl.Name)
			}
			playlistNames[pl.Name] = true
		}
	}

	for i, epg := range c.EPGs {
		if err := epg.Validate(); err != nil {
			return fmt.Errorf("epg[%d] validation failed: %w", i, err)
		}
		if epg.Name != "" {
			if epgNames[epg.Name] {
				return fmt.Errorf("duplicate EPG name: %s", epg.Name)
			}
			epgNames[epg.Name] = true
		}
	}

	clientNames := make(map[string]bool)
	clientSecrets := make(map[string][]string)

	for i, client := range c.Clients {
		if err := client.Validate(playlistNames, epgNames); err != nil {
			return fmt.Errorf("client[%d] validation failed: %w", i, err)
		}

		if clientNames[client.Name] {
			return fmt.Errorf("duplicate client name: %s", client.Name)
		}
		clientNames[client.Name] = true

		if client.Secret != "" {
			if existingClients, exists := clientSecrets[client.Secret]; exists {
				allClients := append(existingClients, client.Name)
				clientSecrets[client.Secret] = allClients
				return fmt.Errorf("duplicate secret: %v", allClients)
			} else {
				clientSecrets[client.Secret] = []string{client.Name}
			}
		}
	}

	for i, rule := range c.ChannelRules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("channel_rules[%d] validation failed: %w", i, err)
		}
		if err := c.validateChannelRuleReferences(rule, clientNames, playlistNames); err != nil {
			return fmt.Errorf("channel_rules[%d] reference validation failed: %w", i, err)
		}
	}

	for i, rule := range c.PlaylistRules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("playlist_rules[%d] validation failed: %w", i, err)
		}
		if err := c.validatePlaylistRuleReferences(rule, clientNames, playlistNames); err != nil {
			return fmt.Errorf("playlist_rules[%d] reference validation failed: %w", i, err)
		}
	}

	return nil
}

func (c *Config) validateChannelRuleReferences(rule *channel.Rule, clientNames, playlistNames map[string]bool) error {
	if rule.SetField != nil && rule.SetField.Condition != nil {
		return c.validateConditionReferences(*rule.SetField.Condition, clientNames, playlistNames)
	}
	if rule.RemoveField != nil && rule.RemoveField.Condition != nil {
		return c.validateConditionReferences(*rule.RemoveField.Condition, clientNames, playlistNames)
	}
	if rule.RemoveChannel != nil && rule.RemoveChannel.Condition != nil {
		return c.validateConditionReferences(*rule.RemoveChannel.Condition, clientNames, playlistNames)
	}
	if rule.MarkHidden != nil && rule.MarkHidden.Condition != nil {
		return c.validateConditionReferences(*rule.MarkHidden.Condition, clientNames, playlistNames)
	}
	return nil
}

func (c *Config) validatePlaylistRuleReferences(rule *playlist.Rule, clientNames, playlistNames map[string]bool) error {
	if rule.MergeChannels != nil && rule.MergeChannels.Condition != nil {
		return c.validateConditionReferences(*rule.MergeChannels.Condition, clientNames, playlistNames)
	}
	if rule.RemoveDuplicates != nil && rule.RemoveDuplicates.Condition != nil {
		return c.validateConditionReferences(*rule.RemoveDuplicates.Condition, clientNames, playlistNames)
	}
	if rule.SortRule != nil && rule.SortRule.Condition != nil {
		return c.validateConditionReferences(*rule.SortRule.Condition, clientNames, playlistNames)
	}
	return nil
}

func (c *Config) validateConditionReferences(condition common.Condition, clientNames, playlistNames map[string]bool) error {
	for _, clientName := range condition.Clients {
		if !clientNames[clientName] {
			return fmt.Errorf("rule references unknown client: %s", clientName)
		}
	}

	for _, playlistName := range condition.Playlists {
		if !playlistNames[playlistName] {
			return fmt.Errorf("rule references unknown playlist: %s", playlistName)
		}
	}

	for _, andCondition := range condition.And {
		if err := c.validateConditionReferences(andCondition, clientNames, playlistNames); err != nil {
			return err
		}
	}

	for _, orCondition := range condition.Or {
		if err := c.validateConditionReferences(orCondition, clientNames, playlistNames); err != nil {
			return err
		}
	}

	return nil
}
