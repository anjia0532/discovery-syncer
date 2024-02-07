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
	"errors"
	"fmt"
	"github.com/anjia0532/apisix-discovery-syncer/model"
	"github.com/ghodss/yaml"
	go_logger "github.com/phachon/go-logger"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"
)

type ApisixClient struct {
	Client        http.Client
	Config        model.Gateway
	ApiVersion    model.ApisixAdminApiVersion
	UpstreamIdMap map[string]string // upstream name
	Logger        *go_logger.Logger
	mutex         sync.Mutex
}

var fetchAllUpstream = "upstreams"

var filePath = filepath.Join(os.TempDir(), "apisix.yaml")

var ApisixConfigTemplate = `
# Auto generate by https://github.com/anjia0532/discovery-syncer, Don't Modify

# apisix 2.x modify conf/config.yaml https://apisix.apache.org/docs/apisix/2.15/stand-alone/
# apisix:
#  enable_admin: false
#  config_center: yaml

# apisix 3.x modify conf/config.yaml https://apisix.apache.org/docs/apisix/3.2/deployment-modes/#standalone
# deployment:
#  role: data_plane
#  role_data_plane:
#    config_provider: yaml

# save as conf/apisix.yaml

{{.Value}}
#END
`

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

func (apisixClient *ApisixClient) GetServiceAllInstances(upstreamName string) ([]model.Instance, error) {
	apisixClient.mutex.Lock()
	if apisixClient.UpstreamIdMap == nil {
		apisixClient.UpstreamIdMap = make(map[string]string)
	}
	upstreamId, ok := apisixClient.UpstreamIdMap[upstreamName]
	if !ok {
		upstreamId = fetchAllUpstream
	}

	instances := []model.Instance{}
	aNode, _, err := apisixClient.httpDo(upstreamId, "GET", nil)

	if err != nil {
		apisixClient.Logger.Errorf("fetch apisix upstream: %s failed", upstreamId)
		return instances, err
	}

	for _, node := range aNode.AList {
		if nil == node.Value {
			continue
		}
		upstream := model.AUpstream{
			Id:   node.Value["id"].(string),
			Name: node.Value["name"].(string),
		}
		for _, n := range node.Value["nodes"].([]interface{}) {
			upstream.Nodes = append(upstream.Nodes, n.(map[string]interface{}))
		}
		apisixClient.UpstreamIdMap[upstream.Name] = fmt.Sprintf("%s/%s", fetchAllUpstream, upstream.Id)
		if upstreamName != upstream.Name {
			continue
		}
		for _, n := range upstream.Nodes {
			instance := model.Instance{Weight: float32(n["weight"].(float64)), Ip: n["host"].(string), Port: n["port"].(int)}
			instances = append(instances, instance)
		}
	}
	apisixClient.mutex.Unlock()
	apisixClient.Logger.Debugf("fetch apisix upstream:%s,instances:%#v", upstreamId, instances)
	return instances, nil
}

func (apisixClient *ApisixClient) SyncInstances(name string, tpl string, discoveryInstances []model.Instance,
	diffIns []model.Instance) error {
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
			apisixClient.Logger.Errorf("parse apisix UpstreamTemplate failed,tmpl:%s,data:%#v", tmpl, data)
		} else {
			body = buf.String()
		}
	} else {
		upstreamId = upstreamId + "/nodes"
		body = string(nodesJson)
	}

	respRawByte, url, _ := apisixClient.httpDoRaw(upstreamId, method, bytes.NewBufferString(body))

	apisixClient.Logger.Debugf("update apisix upstream uri:%s,method:%s,body:%s,resp:%s",
		url, method, body, respRawByte)

	if err != nil {
		apisixClient.Logger.Errorf("update apisix upstream uri:%s,method:%s,body:%s,resp:%s failed",
			url, method, body, respRawByte)
		return err
	}

	return err
}

func (apisixClient *ApisixClient) FetchAdminApiToFile() (string, string, error) {
	var tpl bytes.Buffer

	apisixConfig := model.ApisixConfig{}

	for uri, field := range model.ApisixUris {
		if !slices.Contains(field.Version, apisixClient.ApiVersion) {
			continue
		}

		Nodes, err := apisixClient.fetchInfoFromApisix(uri)

		if err != nil {
			apisixClient.Logger.Errorf("[admin_api_to_yaml]fetchInfoFromApisix error,uri,%s,err:%s", uri, err)
			continue
		}

		v := reflect.ValueOf(&apisixConfig).Elem()
		if f := v.FieldByName(field.Field); f.IsValid() {
			f.Set(reflect.ValueOf(Nodes))
		}
	}

	ymlBytes, err := yaml.Marshal(apisixConfig)
	if err != nil {
		apisixClient.Logger.Errorf("[admin_api_to_yaml]convert json to yaml error,err:%s", err)
		return "", "", err
	}

	tmpl, err := template.New("ApisixConfigTemplate").Parse(ApisixConfigTemplate)
	if err != nil {
		apisixClient.Logger.Errorf("[admin_api_to_yaml]parse template error,err:%s", err)
		return "", "", err
	}

	value := map[string]string{"Value": string(ymlBytes)}
	err = tmpl.Execute(&tpl, value)
	if err != nil {
		apisixClient.Logger.Errorf("[admin_api_to_yaml]template execute error,err:%s", err)
		return "", "", err
	}

	err = os.WriteFile(filePath, tpl.Bytes(), 0644)
	if err != nil {
		apisixClient.Logger.Errorf("[admin_api_to_yaml]failed to write apisix.yaml ,err:%s", err)
		return "", "", err
	}
	return tpl.String(), filePath, nil
}

var (
	ignoreUris = []string{"plugins/list"}
	aliaseUrls = map[string]string{"ssl": "ssls", "proto": "protos", "ssls": "ssl", "protos": "proto"}
)

func (apisixClient *ApisixClient) MigrateTo(gateway GatewayClient) error {
	// 拉取 origin 网关配置
	// 拉取 目标 网关配置
	// 仅创建/创建或更新
	targetApisixClient, ok := gateway.(*ApisixClient)
	if !ok {
		return errors.New("Target GatewayClient is not ApisixClient")
	}

	for uri, field := range model.ApisixUris {
		if slices.Contains(ignoreUris, uri) || !slices.Contains(field.Version, apisixClient.ApiVersion) {
			continue
		}
		// 拉取原网关数据
		resp, url, err := apisixClient.httpDo(uri, "GET", nil)
		if err != nil {
			apisixClient.Logger.Errorf("[migrate]fetch origin apisix info error,url:%s,err:%s", url, err.Error())
			continue
		}
		apisixClient.Logger.Infof("fetch origin apisix info success,url:%s,%#v", url, resp)

		auri, ok := aliaseUrls[uri]

		if !ok {
			auri = uri
		}

		for _, node := range resp.AList {
			if nil == node.Value {
				continue
			}
			reqBody, _ := node.Translate(targetApisixClient.ApiVersion)
			id := node.Value["id"].(string)
			// 更新或者创建
			respBody, url, err := targetApisixClient.httpDoRaw(auri+"/"+id, "PUT", bytes.NewReader(reqBody))
			if err != nil {
				apisixClient.Logger.Errorf("[migrate]create target apisix info error,url:%s,err:%s", url, err.Error())
				continue
			}
			apisixClient.Logger.Infof("[migrate]save or update target apisix info,url:%s, %s", url, respBody)
		}
	}
	return nil
}

func (apisixClient *ApisixClient) fetchInfoFromApisix(uri string) ([]map[string]interface{}, error) {

	var plugins []string
	var url string
	var err error
	aNode := model.ANode{}

	if strings.Contains(uri, "plugins/list") {
		var resp []byte
		resp, url, err = apisixClient.httpDoRaw(uri, "GET", nil)
		plugins = []string{}
		err = json.Unmarshal(resp, &plugins)
		for _, plugin := range plugins {
			// TODO /apisix/admin/plugins?all=true&subsystem=stream 会返回所有stream plugins
			aNode.AList = append(aNode.AList, model.ANode{Value: map[string]interface{}{"name": plugin}})
		}
	} else {
		aNode, url, err = apisixClient.httpDo(uri, "GET", nil)
	}

	if err != nil {
		apisixClient.Logger.Errorf("[admin_api_to_yaml]fetch apisix info error,url:%s,err:%s", url, err.Error())
		return nil, err
	}
	nodes := []map[string]interface{}{}
	for _, node := range aNode.AList {
		if nil == node.Value {
			continue
		}
		nodes = append(nodes, node.Value)
	}
	return nodes, nil
}

func (apisixClient *ApisixClient) httpDoRaw(uri string, method string, body io.Reader) ([]byte, string, error) {
	url := apisixClient.Config.AdminUrl + apisixClient.Config.Prefix + uri
	hc := &http.Client{Timeout: 30 * time.Second}

	req, _ := http.NewRequest(method, url, body)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("X-API-KEY", apisixClient.Config.Config["X-API-KEY"])
	resp, err := hc.Do(req)
	var respBytes []byte
	if err != nil {
		apisixClient.Logger.Errorf("access apisix error,%s", url)
		return []byte{}, url, err
	}
	respBytes, _ = io.ReadAll(resp.Body)
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			apisixClient.Logger.Errorf("close resp.Body error,%s", err.Error())
		}
	}(resp.Body)
	return respBytes, url, nil
}

func (apisixClient *ApisixClient) httpDo(uri string, method string, body io.Reader) (model.ANode, string, error) {
	respBody, url, err := apisixClient.httpDoRaw(uri, method, body)
	resp := model.ANode{}
	err = resp.UnmarshalJSON(respBody, apisixClient.ApiVersion)
	if err != nil {
		apisixClient.Logger.Errorf("apisix respone decode error,%s,%s", url, err.Error())
		return model.ANode{}, url, err
	}
	return resp, url, nil
}
