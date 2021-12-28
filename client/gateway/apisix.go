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

package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/anjia0532/apisix-discovery-syncer/config"
	"github.com/anjia0532/apisix-discovery-syncer/dto"
	go_logger "github.com/phachon/go-logger"
	"io"
	"net/http"
	"sync"
	"text/template"
)

type ApisixClient struct {
	Client        http.Client
	Config        config.Gateway
	UpstreamIdMap map[string]string
	Logger        *go_logger.Logger
	mutex         sync.Mutex
}

var fetchAllUpstream = "upstreams"

func (apisixClient *ApisixClient) GetServiceAllInstances(upstreamName string) ([]dto.Instance, error) {
	apisixClient.mutex.Lock()
	if apisixClient.UpstreamIdMap == nil {
		apisixClient.UpstreamIdMap = make(map[string]string)
	}
	upstreamId, ok := apisixClient.UpstreamIdMap[upstreamName]
	if !ok {
		upstreamId = fetchAllUpstream
	}

	uri := apisixClient.Config.AdminUrl + apisixClient.Config.Prefix + upstreamId
	hc := &http.Client{}

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("X-API-KEY", apisixClient.Config.Config["X-API-KEY"])
	resp, err := hc.Do(req)

	if err != nil {
		apisixClient.Logger.Errorf("fetch apisix upstream error,%s", uri)
		return nil, err
	}

	apisixResp := ApisixNodeResp{}
	err = json.NewDecoder(resp.Body).Decode(&apisixResp)
	_ = resp.Body.Close()
	if err != nil {
		apisixClient.Logger.Errorf("fetch apisix upstream and decode json error,%s", uri, err)
		return nil, err
	}

	instances := []dto.Instance{}
	if upstreamId != fetchAllUpstream {
		apisixResp.Node.Nodes = append(apisixResp.Node.Nodes, apisixResp.Node)
	}
	for _, node := range apisixResp.Node.Nodes {
		apisixClient.UpstreamIdMap[node.Value.Name] = fmt.Sprintf("%s/%s", fetchAllUpstream, node.Value.Id)
		if upstreamName != node.Value.Name {
			continue
		}
		//for host, weight := range node.Value.Nodes {
		//	ts := strings.Split(host, ":")
		//	p, _ := strconv.Atoi(ts[1])
		//	instance := dto.Instance{Weight: float32(weight), Ip: ts[0], Port: p}
		//	instances = append(instances, instance)
		//}
	}
	apisixClient.mutex.Unlock()
	apisixClient.Logger.Debugf("fetch apisix upstream:%s,instances:%+v", uri, instances)
	return instances, nil
}

var DefaultApisixUpstreamTemplate = `
{
    "timeout": {
        "connect": 30,
        "send": 30,
        "read": 30
    },
    "name": "{{.Name}}",
    "nodes": {{.Nodes}},
    "type":"roundrobin",
    "desc": "auto sync by https://github.com/anjia0532/discovery-syncer"
}
`

func (apisixClient *ApisixClient) SyncInstances(name string, tpl string, discoveryInstances []dto.Instance,
	diffIns []dto.Instance) error {
	if len(diffIns) == 0 && len(discoveryInstances) == 0 {
		return nil
	}
	//apisix 不支持变量更新nodes，所以diffIns无用，直接用discoveryInstances即可
	method := "PATCH"
	upstreamId, ok := apisixClient.UpstreamIdMap[name]

	nodes := map[string]float32{}
	for _, instance := range discoveryInstances {
		nodes[fmt.Sprintf("%s:%d", instance.Ip, instance.Port)] = instance.Weight
	}
	nodesJson, err := json.Marshal(nodes)
	var body string
	if !ok {
		method = "PUT"
		upstreamId = fetchAllUpstream + "/" + name
		if len(tpl) == 0 {
			tpl = DefaultApisixUpstreamTemplate
		}
		tmpl, err := template.New("UpstreamTemplate").Parse(tpl)
		if err != nil {
			apisixClient.Logger.Errorf("parse apisix UpstreamTemplate failed,tmpl:%s", tmpl)
			return err
		}
		var buf bytes.Buffer
		data := struct {
			Name  string
			Nodes string
		}{Name: name, Nodes: string(nodesJson)}
		err = tmpl.Execute(&buf, data)
		if err != nil {
			apisixClient.Logger.Errorf("parse apisix UpstreamTemplate failed,tmpl:%s,data:%+v", tmpl, data)
		} else {
			body = buf.String()
		}
	} else {
		upstreamId = upstreamId + "/nodes"
		body = string(nodesJson)
	}

	uri := apisixClient.Config.AdminUrl + apisixClient.Config.Prefix + upstreamId
	hc := &http.Client{}

	req, _ := http.NewRequest(method, uri, bytes.NewBufferString(body))

	req.Header.Add("Accept", "application/json")
	req.Header.Add("X-API-KEY", apisixClient.Config.Config["X-API-KEY"])
	resp, err := hc.Do(req)

	respRawByte, _ := io.ReadAll(resp.Body)

	apisixClient.Logger.Debugf("update apisix upstream uri:%s,method:%s,body:%s,resp:%s",
		uri, method, body, respRawByte)

	if err != nil {
		apisixClient.Logger.Errorf("update apisix upstream uri:%s,method:%s,body:%s,resp:%s failed",
			uri, method, body, respRawByte)
		return err
	}
	if resp.StatusCode >= 400 {
		apisixClient.Logger.Errorf("update apisix upstream uri:%s,method:%s,body:%s,resp:%s failed",
			uri, method, body, respRawByte)
		return nil
	}
	_ = resp.Body.Close()
	return err
}

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
