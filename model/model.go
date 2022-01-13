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

package model

import (
	"encoding/json"
	"strings"
)

type Service struct {
	Name      string
	Instances []Instance
}

type Instance struct {
	Port     int               `json:"port"`
	Ip       string            `json:"ip"`
	Weight   float32           `json:"weight"`
	Metadata map[string]string `json:"metadata"`
	Enabled  bool              `json:"enabled,omitempty"`
	Change   bool              `json:"change"`
	Ext      map[string]string `json:"ext"`
}

type GetInstanceVo struct {
	ServiceName string
	ExtData     map[string]string
}

type Registration struct {
	Type        string            `json:"type,omitempty"` // "METADATA" "IP"
	RegexpStr   string            `json:"regexpStr,omitempty"`
	MetadataKey string            `json:"metadataKey,omitempty"`
	Status      string            `json:"status,omitempty"`      //"UP" "DOWN"
	OtherStatus string            `json:"otherStatus,omitempty"` //"UP" "DOWN" "ORIGIN"
	ServiceName string            `json:"serviceName,omitempty"`
	ExtData     map[string]string `json:"extData,omitempty"`
}

func (c *Registration) UnmarshalJSON(data []byte) error {
	*c = Registration{}

	type plain Registration
	if err := json.Unmarshal(data, (*plain)(c)); err != nil {
		return err
	}

	// default other status is keep origin
	c.OtherStatus = strings.ToUpper(c.OtherStatus)
	switch c.OtherStatus {
	case "UP", "DOWN":
		break
	default:
		c.OtherStatus = "ORIGIN"
	}
	// default status is UP
	c.Status = strings.ToUpper(c.Status)
	if c.Status != "DOWN" {
		c.Status = "UP"
	}
	c.Type = strings.ToUpper(c.Type)
	switch c.Type {
	case "METADATA", "IP":
		break
	default:
		c.Type = "METADATA"
	}
	return nil
}
