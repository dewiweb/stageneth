// Package stcfg loads /etc/config/stageneth into a typed Config.
package stcfg

import (
	"os"
	"sort"

	"stageneth/internal/ucifile"
)

// Config is the parsed StageNeth source of truth.
type Config struct {
	SchemaVersion string                       `json:"schema_version"`
	Globals       map[string]string            `json:"globals"`
	Services      map[string]map[string]string `json:"services"`
	Bindings      map[string]map[string]string `json:"bindings"`
	Forwardings   map[string]map[string]string `json:"forwardings"`
	Logging       map[string]map[string]string `json:"logging"`
}

func optionMap(s *ucifile.Section) map[string]string {
	m := map[string]string{}
	lists := map[string][]string{}
	for _, o := range s.Options {
		if o.List {
			lists[o.Name] = append(lists[o.Name], o.Value)
		} else {
			m[o.Name] = o.Value
		}
	}
	for k, v := range lists {
		m[k] = joinSpace(v)
	}
	return m
}

func joinSpace(v []string) string {
	out := ""
	for i, s := range v {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}

// Parse builds a Config from a parsed stageneth UCI file.
func Parse(f *ucifile.File) *Config {
	c := &Config{
		Globals:     map[string]string{},
		Services:    map[string]map[string]string{},
		Bindings:    map[string]map[string]string{},
		Forwardings: map[string]map[string]string{},
		Logging:     map[string]map[string]string{},
	}
	for i := range f.Sections {
		s := &f.Sections[i]
		switch s.Type {
		case "stageneth":
			if s.Name == "globals" {
				c.Globals = optionMap(s)
			}
		case "service":
			c.Services[s.Name] = optionMap(s)
		case "binding":
			c.Bindings[s.Name] = optionMap(s)
		case "forwarding":
			c.Forwardings[s.Name] = optionMap(s)
		case "logging":
			c.Logging[s.Name] = optionMap(s)
		}
	}
	c.SchemaVersion = c.Globals["schema_version"]
	return c
}

// Load reads and parses the stageneth config file at path.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(ucifile.Parse(string(b))), nil
}

// ServiceNames returns sorted service section names.
func (c *Config) ServiceNames() []string {
	names := make([]string, 0, len(c.Services))
	for n := range c.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// SortedBindings returns binding option maps in deterministic name order.
func (c *Config) SortedBindings() []map[string]string {
	names := make([]string, 0, len(c.Bindings))
	for n := range c.Bindings {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]map[string]string, 0, len(names))
	for _, n := range names {
		out = append(out, c.Bindings[n])
	}
	return out
}
