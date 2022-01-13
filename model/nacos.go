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

type NacosInstanceResp struct {
	Hosts []NacosInstance `json:"hosts"`
}

type NacosInstance struct {
	Port        int               `json:"port"`
	Ip          string            `json:"ip"`
	Weight      float32           `json:"weight"`
	Metadata    map[string]string `json:"metadata"`
	Enabled     bool              `json:"enabled,omitempty"`
	Ephemeral   bool              `json:"ephemeral,omitempty"`
	NamespaceId string            `json:"namespaceId,omitempty"`
	ClusterName string            `json:"clusterName,omitempty"`
	GroupName   string            `json:"groupName,omitempty"`
	ServiceName string            `json:"serviceName,omitempty"`
}
type NacosServiceResp struct {
	ServiceNames []string `json:"doms"`
	Total        int      `json:"count"`
}
