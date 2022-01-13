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
	"fmt"
)

type ApisixUpstream struct {
	Id     string      `json:"id"`
	TNodes interface{} `json:"nodes"`
	Nodes  map[string]float64
	Name   string `json:"name"`
}

type ApisixNode struct {
	Nodes []ApisixNode   `json:"nodes"`
	Value ApisixUpstream `json:"value"`
}

type ApisixNodeResp struct {
	Node ApisixNode `json:"node"`
}

func (c *ApisixNodeResp) UnmarshalJSON(data []byte) error {
	*c = ApisixNodeResp{}

	type plain ApisixNodeResp
	if err := json.Unmarshal(data, (*plain)(c)); err != nil {
		return err
	}

	for _, node := range c.Node.Nodes {
		node.Value.Nodes = make(map[string]float64)
		// TNodes interface{} `json:"nodes"`
		switch node.Value.TNodes.(type) {
		case []interface{}:
			//[
			//    {
			//      "host": "10.42.113.174",
			//      "port": 9090,
			//      "weight": 2
			//    }
			//  ]
			tArr := node.Value.TNodes.([]interface{})
			for _, arr := range tArr {
				myMap := arr.(map[string]interface{})
				node.Value.Nodes[fmt.Sprintf("%s:%.0f", myMap["host"], myMap["port"])] = myMap["weight"].(float64)
			}

		case map[string]interface{}:
			// {
			//    "10.42.163.208:8099": 1
			//  }
			myMap := node.Value.TNodes.(map[string]interface{})
			for k, v := range myMap {
				node.Value.Nodes[k] = v.(float64)
			}
		}
	}

	return nil
}
