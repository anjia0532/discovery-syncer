/*
 * Copyright (c) 2022 The AnJia Authors.
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

package model

import (
	"encoding/json"
	"strconv"
	"strings"
)

type ApisixConfig struct {
	Routes         interface{} `json:"routes,omitempty" yaml:"routes,omitempty"`
	Services       interface{} `json:"services,omitempty" yaml:"services,omitempty"`
	Consumers      interface{} `json:"consumers,omitempty" yaml:"consumers,omitempty"`
	Upstreams      interface{} `json:"upstreams,omitempty" yaml:"upstreams,omitempty"`
	Ssl            interface{} `json:"ssl,omitempty" yaml:"ssl,omitempty"`
	Ssls           interface{} `json:"ssls,omitempty" yaml:"ssls,omitempty"`
	GlobalRules    interface{} `json:"global_rules,omitempty" yaml:"global_rules,omitempty"`
	ConsumerGroups interface{} `json:"consumer_groups,omitempty" yaml:"consumer_groups,omitempty"`
	Plugins        interface{} `json:"plugins,omitempty" yaml:"plugins,omitempty"`
	PluginConfigs  interface{} `json:"plugin_configs,omitempty" yaml:"plugin_configs,omitempty"`
	PluginMetadata interface{} `json:"plugin_metadata,omitempty" yaml:"plugin_metadata,omitempty"`
	StreamRoutes   interface{} `json:"stream_routes,omitempty" yaml:"stream_routes,omitempty"`
	Secrets        interface{} `json:"secrets,omitempty" yaml:"secrets,omitempty"`
	Protos         interface{} `json:"protos,omitempty" yaml:"protos,omitempty"`
	Proto          interface{} `json:"proto,omitempty" yaml:"protos,omitempty"`
}
type ApisixStruct struct {
	Version []ApisixAdminApiVersion
	Field   string
}

var ApisixUris = map[string]ApisixStruct{
	"routes":          {Version: []ApisixAdminApiVersion{APISIX_V2, APISIX_V3}, Field: "Routes"},
	"services":        {Version: []ApisixAdminApiVersion{APISIX_V2, APISIX_V3}, Field: "Services"},
	"consumers":       {Version: []ApisixAdminApiVersion{APISIX_V2, APISIX_V3}, Field: "Consumers"},
	"upstreams":       {Version: []ApisixAdminApiVersion{APISIX_V2, APISIX_V3}, Field: "Upstreams"},
	"ssl":             {Version: []ApisixAdminApiVersion{APISIX_V2}, Field: "Ssl"},
	"ssls":            {Version: []ApisixAdminApiVersion{APISIX_V3}, Field: "Ssls"},
	"global_rules":    {Version: []ApisixAdminApiVersion{APISIX_V2, APISIX_V3}, Field: "GlobalRules"},
	"consumer_groups": {Version: []ApisixAdminApiVersion{APISIX_V3}, Field: "ConsumerGroups"},
	"plugin_configs":  {Version: []ApisixAdminApiVersion{APISIX_V2, APISIX_V3}, Field: "PluginConfigs"},
	"plugin_metadata": {Version: []ApisixAdminApiVersion{APISIX_V2, APISIX_V3}, Field: "PluginMetadata"},
	"plugins/list":    {Version: []ApisixAdminApiVersion{APISIX_V2, APISIX_V3}, Field: "Plugins"},
	"stream_routes":   {Version: []ApisixAdminApiVersion{APISIX_V2, APISIX_V3}, Field: "StreamRoutes"},
	"secrets":         {Version: []ApisixAdminApiVersion{APISIX_V3}, Field: "Secrets"},
	"protos":          {Version: []ApisixAdminApiVersion{APISIX_V3}, Field: "Protos"},
	"proto":           {Version: []ApisixAdminApiVersion{APISIX_V2}, Field: "Proto"},
}

var hasUpstream = []string{"/apisix/routes/", "/apisix/stream_routes/", "/apisix/services/", "/apisix/upstreams/"}

type ANode struct {
	Value   map[string]interface{} `json:"value,omitempty" yaml:"value,omitempty"` // detail
	Key     string                 `json:"key,omitempty" yaml:"key,omitempty"`     // 通过 key 判断如何解析
	Nodes   []ANode                `json:"nodes,omitempty" yaml:"nodes,omitempty"` // v2 list
	List    []ANode                `json:"list,omitempty" yaml:"list,omitempty"`   // v3 list
	Node    *ANode                 `json:"node,omitempty" yaml:"node"`
	Version ApisixAdminApiVersion  `json:"-"`
	AList   []ANode                `json:"-"`
}

func (node *ANode) Translate(version ApisixAdminApiVersion) ([]byte, error) {
	if node.Version == version {
		return json.Marshal(node.Value)
	}
	// https://apisix.apache.org/docs/apisix/upgrade-guide-from-2.15.x-to-3.0.0/
	if APISIX_V2 == node.Version && APISIX_V3 == version {
		// v2->v3
		//plugin.disable -> plugin._meta.disable
		plugins, ok := node.Value["plugins"]
		if ok {
			for key, plugin := range plugins.(map[string]interface{}) {
				disable, hasKey := plugin.(map[string]interface{})["disable"]
				if hasKey {
					node.Value["plugins"].(map[string]interface{})[key].(map[string]interface{})["_meta"] = map[string]interface{}{"disable": disable}
					delete(node.Value["plugins"].(map[string]interface{})[key].(map[string]interface{}), "disable")
				}
			}
		}
		//route.service_protocol -> route.upstream.scheme
		_, ok = node.Value["upstream"]
		if ok {
			protocol, hasProtocol := node.Value["service_protocol"]
			if hasProtocol {
				node.Value["upstream"].(map[string]interface{})["scheme"] = protocol
				delete(node.Value, "service_protocol")
			}
		}
	} else if APISIX_V3 == node.Version && APISIX_V2 == version {
		// v3->v2
		//plugin._meta.disable -> plugin.disable
		plugins, ok := node.Value["plugins"]
		if ok {
			for key, plugin := range plugins.(map[string]interface{}) {
				if plugin.(map[string]interface{})["_meta"] != nil && plugin.(map[string]interface{})["_meta"].(map[string]interface{})["disable"] != nil {
					node.Value["plugins"].(map[string]interface{})[key].(map[string]interface{})["disable"] = plugin.(map[string]interface{})["_meta"].(map[string]interface{})["disable"]
					delete(node.Value["plugins"].(map[string]interface{})[key].(map[string]interface{})["_meta"].(map[string]interface{}), "service_protocol")
				}
			}
		}
		//route.upstream.scheme -> route.service_protocol
		upstream, ok := node.Value["upstream"]
		if ok {
			scheme, hasScheme := upstream.(map[string]interface{})["scheme"]
			if hasScheme {
				node.Value["service_protocol"] = scheme
				delete(node.Value["upstream"].(map[string]interface{}), "scheme")
			}
		}
	}
	return json.Marshal(node.Value)
}

type AUpstream struct {
	Nodes []map[string]interface{} `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Id    string                   `json:"id,omitempty" yaml:"id,omitempty"`
	Name  string                   `json:"name,omitempty" yaml:"name,omitempty"`
}

func (c *ANode) UnmarshalJSON(data []byte, Version ApisixAdminApiVersion) error {
	type plain ANode

	if err := json.Unmarshal(data, (*plain)(c)); err != nil {
		return err
	}

	c.Version = Version
	var nodes []ANode
	if APISIX_V2 == Version {
		// v2
		if nil == c.Node {
			return nil
		} else if len(c.Node.Nodes) == 0 {
			nodes = append(nodes, ANode{Key: c.Node.Key, Value: c.Node.Value})
		} else {
			nodes = append(nodes, c.Node.Nodes...)
		}
	} else {
		// v3
		if len(c.List) == 0 {
			nodes = append(nodes, ANode{Key: c.Key, Value: c.Value})
		} else {
			nodes = append(nodes, c.List...)
		}
	}

	for _, node := range nodes {
		node.Version = Version
		delete(node.Value, "update_time")
		delete(node.Value, "create_time")
		delete(node.Value, "validity_end")
		delete(node.Value, "validity_start")
		for _, k := range hasUpstream {
			if strings.HasPrefix(node.Key, k) {
				var upNodes interface{}
				var ok bool
				var upstream map[string]interface{}
				if strings.HasPrefix(node.Key, "/apisix/upstreams/") {
					upstream = node.Value
				} else {
					upstream, ok = node.Value["upstream"].(map[string]interface{})
					if !ok {
						continue
					}
				}
				upNodes, ok = upstream["nodes"].(map[string]interface{})
				if !ok {
					continue
				}
				var tNodes []interface{}
				switch upNodes.(type) {
				//case []interface{}:
				//[
				//    {
				//      "host": "10.42.113.174",
				//      "port": 9090,
				//      "weight": 2
				//    }
				//  ]
				//tNodes = append(tNodes, upNodes.([]interface{})...)
				case map[string]interface{}:
					// {
					//    "10.42.163.208:8099": 1
					//  }
					tMap := upNodes.(map[string]interface{})
					for k, v := range tMap {
						ts := strings.Split(k, ":")
						p, _ := strconv.Atoi(ts[1])
						tNodes = append(tNodes, map[string]interface{}{"host": ts[0], "port": p, "weight": v})
					}
					upstream["nodes"] = tNodes
					if !strings.HasPrefix(node.Key, "/apisix/upstreams/") {
						node.Value["upstream"] = upstream
					}
				}
				break
			}
		}
		c.AList = append(c.AList, node)
	}
	return nil
}
