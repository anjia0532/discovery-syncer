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
	"strconv"
	"strings"
	"text/template"
)

type KongClient struct {
	Client        http.Client
	Config        config.Gateway
	Logger        *go_logger.Logger
	UpstreamIdMap map[string]int
}

func (kongClient *KongClient) GetServiceAllInstances(upstreamName string) ([]dto.Instance, error) {
	if len(kongClient.UpstreamIdMap) == 0 {
		kongClient.UpstreamIdMap = make(map[string]int)
	}
	uri := kongClient.Config.AdminUrl + kongClient.Config.Prefix + upstreamName + "/targets/all/"
	hc := &http.Client{}

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	resp, err := hc.Do(req)

	if err != nil {
		kongClient.Logger.Errorf("fetch kong upstream error,%s", uri)
		return nil, err
	}
	kongClient.UpstreamIdMap[upstreamName] = resp.StatusCode
	if resp.StatusCode == 404 {
		return make([]dto.Instance, 0), nil
	}

	kongResp := KongTargetResp{}
	err = json.NewDecoder(resp.Body).Decode(&kongResp)
	_ = resp.Body.Close()
	if err != nil {
		kongClient.Logger.Errorf("fetch kong upstream and decode json error,%s", uri, err)
		return nil, err
	}

	instances := []dto.Instance{}
	for _, target := range kongResp.Data {
		parts := strings.Split(target.Target, ":")
		port, _ := strconv.Atoi(parts[1])
		instances = append(instances, dto.Instance{Weight: target.Weight, Ip: parts[0], Port: port})
	}
	return instances, nil
}

var DefaultKongUpstreamTemplate = `
{
    "name": "{{.Name}}",
    "tags": ["discovery-syncer-auto"]
}
`
var DefaultKongTargetTemplate = `
{
    "target": "%s:%d",
    "weight": %.0f,
    "tags": ["discovery-syncer-auto"]
}
`

func (kongClient *KongClient) SyncInstances(name string, tpl string, discoveryInstances []dto.Instance,
	diffIns []dto.Instance) error {

	if len(diffIns) == 0 && len(discoveryInstances) == 0 {
		return nil
	}
	var (
		buf  bytes.Buffer
		body string
	)
	uri := kongClient.Config.AdminUrl + kongClient.Config.Prefix + name
	hc := &http.Client{}

	// added new upstream
	if kongClient.UpstreamIdMap[name] == 404 {

		if len(tpl) == 0 {
			tpl = DefaultKongUpstreamTemplate
		}

		tmpl, err := template.New("UpstreamKongTemplate").Parse(tpl)
		if err != nil {
			kongClient.Logger.Errorf("parse kong UpstreamTemplate failed,tmpl:%s", tmpl)
			return err
		}
		data := map[string]string{"Name": name}
		err = tmpl.Execute(&buf, map[string]string{"Name": name})

		if err != nil {
			kongClient.Logger.Errorf("parse kong UpstreamTemplate failed,tmpl:%s,data:%+v", tmpl, data)
		} else {
			body = buf.String()
		}

		req, _ := http.NewRequest("PUT", uri, bytes.NewBufferString(body))

		req.Header.Add("Accept", "application/json")
		req.Header.Add("Content-Type", "application/json")
		resp, err := hc.Do(req)

		respRawByte, _ := io.ReadAll(resp.Body)

		kongClient.Logger.Debugf("update kong upstream uri:%s,method:POST,body:%s,resp:%s", uri, body,
			respRawByte)
		_ = resp.Body.Close()
	}
	// delete first(delete and patch)
	for _, instance := range diffIns {

		if !instance.Enabled || instance.Change {
			targetUri := uri + "/targets/" + fmt.Sprintf("%s:%d", instance.Ip, instance.Port)
			req, _ := http.NewRequest("DELETE", targetUri, nil)
			req.Header.Add("Accept", "application/json")
			req.Header.Add("Content-Type", "application/json")
			resp, _ := hc.Do(req)
			respRawByte, _ := io.ReadAll(resp.Body)
			kongClient.Logger.Debugf("delete kong target, uri:%s,method:DELETE,body:nil,resp:%s", uri,
				respRawByte)
			_ = resp.Body.Close()
		}

	}
	for _, instance := range diffIns {
		if instance.Enabled {
			targetUri := uri + "/targets/"
			body := fmt.Sprintf(DefaultKongTargetTemplate, instance.Ip, instance.Port, instance.Weight)
			req, _ := http.NewRequest("POST", targetUri, bytes.NewBufferString(body))
			req.Header.Add("Accept", "application/json")
			req.Header.Add("Content-Type", "application/json")
			resp, _ := hc.Do(req)
			respRawByte, _ := io.ReadAll(resp.Body)
			kongClient.Logger.Debugf("added kong target, uri:%s,method:DELETE,body:%s,resp:%s", uri, body,
				respRawByte)
			_ = resp.Body.Close()
		}
	}
	return nil
}

type KongTarget struct {
	Weight float32 `json:"weight"`
	Target string  `json:"target"`
}

type KongTargetResp struct {
	Data []KongTarget `json:"data"`
}
