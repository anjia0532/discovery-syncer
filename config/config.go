/*
 * Copyright (c) 2021 The AnJia Authors.
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *     http://www.apache.org/licenses/LICENSE-2.0
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
)

var (
	HostPatternRE   = regexp.MustCompile(`^http(s)?://(.*@)?[\w-._:]+$`)
	PrefixPatternRE = regexp.MustCompile(`^/[/\w-_.]+/$`)
	NameRE          = regexp.MustCompile(`^[\w-_.]+$`)
)

type DiscoveryType string
type GatewayType string

const (
	NACOS_DISCOVERY  DiscoveryType = "nacos"
	EUREKA_DISCOVERY DiscoveryType = "eureka"

	APISIX_GATEWAY GatewayType = "apisix"
	KONG_GATEWAY   GatewayType = "kong"
)

type Config struct {
	Logger           Logger               `yaml:"logger"`
	DiscoveryServers map[string]Discovery `yaml:"discovery-servers,omitempty"`
	GatewayServers   map[string]Gateway   `yaml:"gateway-servers,omitempty"`
	Targets          []Target             `yaml:"targets,omitempty"`
}
type Logger struct {
	Level     string `yaml:"level"`
	Logger    string `yaml:"logger"`
	LogFile   string `yaml:"log-file"`
	DateSlice string `yaml:"date-slice"`
}

func (c *Logger) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = Logger{}

	type plain Logger
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	var errMsg string
	switch c.Logger {
	case "console", "file":
		errMsg = ""
	default:
		errMsg = fmt.Sprintf("Not support logger:%s,console or file plz", c.Logger)
	}
	if len(errMsg) > 0 {
		return errors.New(errMsg)
	}
	switch c.DateSlice {
	case "y", "m", "d", "h":
	default:
		c.DateSlice = "y"
	}
	return nil
}

type Discovery struct {
	Type   DiscoveryType `yaml:"type,omitempty"`
	Weight float32       `yaml:"weight,omitempty"`
	Prefix string        `yaml:"prefix,omitempty"`
	Host   string        `yaml:"host"`
}

func (c *Discovery) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = Discovery{}

	type plain Discovery
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.Weight < 0 || c.Weight > 100 {
		return errors.New("weight must between 0 ~ 100")
	}
	if len(c.Host) == 0 {
		return errors.New("host must not null")
	}
	if len(c.Prefix) > 0 && !PrefixPatternRE.MatchString(c.Prefix) {
		return errors.New("invalid discovery prefix")
	}
	if !HostPatternRE.MatchString(c.Host) {
		return errors.New("invalid host url")
	}
	switch c.Type {
	case EUREKA_DISCOVERY, NACOS_DISCOVERY:
		return nil
	default:
		return errors.New(fmt.Sprintf("invalid discovery type:%s", c.Type))
	}
}

type Gateway struct {
	Type     GatewayType       `yaml:"type"`
	AdminUrl string            `yaml:"admin-url"`
	Prefix   string            `yaml:"prefix,omitempty"`
	Config   map[string]string `yaml:"config,omitempty"`
}

func (c *Gateway) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = Gateway{}

	type plain Gateway
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if !HostPatternRE.MatchString(c.AdminUrl) {
		return errors.New("invalid gateway admin url")
	}

	if len(c.Prefix) > 0 && !PrefixPatternRE.MatchString(c.Prefix) {
		return errors.New("invalid gateway prefix")
	}

	switch c.Type {
	case APISIX_GATEWAY, KONG_GATEWAY:
		return nil
	default:
		return errors.New(fmt.Sprintf("invalid gateway type:%s", c.Type))
	}
}

type Target struct {
	Discovery      string            `yaml:"discovery,omitempty"`
	Gateway        string            `yaml:"gateway,omitempty"`
	Enabled        bool              `yaml:"enabled,omitempty"`
	ExcludeService []string          `yaml:"exclude-service"`
	UpstreamPrefix string            `yaml:"upstream-prefix"`
	FetchInterval  string            `yaml:"fetch-interval,omitempty"`
	Config         map[string]string `yaml:"config,omitempty"`
}

func (c *Target) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = Target{Enabled: false, FetchInterval: "@every 10s"}

	type plain Target
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if !NameRE.MatchString(c.Discovery) {
		return errors.New("invalid discovery name")
	}
	if !NameRE.MatchString(c.Gateway) {
		return errors.New("invalid gateway name")
	}

	return nil
}

func LoadFile(filename string) (*Config, error) {
	content, err := readConfig(filename)

	cfg := &Config{}
	err = yaml.UnmarshalStrict([]byte(content), cfg)
	if err != nil {
		return nil, err
	}
	if len(cfg.Targets) == 0 {
		return nil, errors.New("targets must not null")
	}
	if len(cfg.DiscoveryServers) == 0 {
		return nil, errors.New("discovery-servers must not null")
	}
	if len(cfg.GatewayServers) == 0 {
		return nil, errors.New("gateway-servers must not null")
	}
	for _, target := range cfg.Targets {
		if _, ok := cfg.DiscoveryServers[target.Discovery]; !ok {
			return nil, errors.New(fmt.Sprintf("discovery %s not exist", target.Discovery))
		}
		if _, ok := cfg.GatewayServers[target.Gateway]; !ok {
			return nil, errors.New(fmt.Sprintf("gateway %s not exist", target.Discovery))
		}
	}
	return cfg, nil
}

func readConfig(filename string) (string, error) {
	if strings.HasPrefix(filename, "http") {
		resp, err := http.DefaultClient.Get(filename)
		if err != nil {
			return "", err
		}
		content, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			return "", err
		}
		_ = resp.Body.Close()
		return string(content), nil
	} else {
		_, err := os.Stat(filename)
		if err != nil {
			return "", err
		}
		content, err := os.ReadFile(filename)
		if err != nil {
			return "", err
		}
		return string(content), nil
	}
}
