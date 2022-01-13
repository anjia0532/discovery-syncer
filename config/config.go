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
	"github.com/anjia0532/apisix-discovery-syncer/model"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

func LoadFile(filename string) (*model.Config, error) {
	content, err := readConfig(filename)

	cfg := &model.Config{}
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
		_, _ = io.Copy(ioutil.Discard, resp.Body)
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
