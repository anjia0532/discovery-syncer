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

package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/anjia0532/apisix-discovery-syncer/model"
	go_logger "github.com/phachon/go-logger"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type NacosClient struct {
	Client http.Client
	Config model.Discovery
	Logger *go_logger.Logger
}

func (nacosClient *NacosClient) GetAllService(data map[string]string) ([]model.Service, error) {
	// /nacos/v1/ns/service/list?pageNo=0&pageSize=100&groupName=&namespaceId=
	data = getDefaultMap(data, map[string]string{
		"pageNo":      "0",
		"pageSize":    "1000000",
		"groupName":   "DEFAULT_GROUP",
		"namespaceId": "",
	})
	r := url.Values{}
	for k, v := range data {
		r.Set(k, v)
	}

	uri := nacosClient.Config.Host + nacosClient.Config.Prefix + "ns/service/list?" + r.Encode()
	resp, err := http.DefaultClient.Get(uri)
	if err != nil {
		nacosClient.Logger.Errorf("fetch nacos service error:%s", uri)
		return nil, errors.New("fetch nacos service error")
	}
	serviceResp := &model.NacosServiceResp{}

	err = json.NewDecoder(resp.Body).Decode(&serviceResp)
	_, _ = io.Copy(ioutil.Discard, resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		nacosClient.Logger.Errorf("fetch nacos service error:%s", uri)
		return nil, errors.New("fetch nacos service error")
	}
	nacosClient.Logger.Debugf("fetch nacos service,uri:%s,%#v", uri, serviceResp)
	services := []model.Service{}
	for _, name := range serviceResp.ServiceNames {
		services = append(services, model.Service{Name: name})
	}
	return services, nil
}

func getDefaultMap(data map[string]string, defaultMap map[string]string) map[string]string {
	for key, val := range defaultMap {
		_, ok := data[key]
		if !ok {
			data[key] = val
		}
	}
	return data
}
func (nacosClient *NacosClient) GetServiceAllInstances(vo model.GetInstanceVo) ([]model.Instance, error) {
	vo.ExtData["serviceName"] = vo.ServiceName
	r := url.Values{}
	for k, v := range vo.ExtData {
		if k == "template" {
			continue
		}
		r.Set(k, v)
	}

	uri := nacosClient.Config.Host + nacosClient.Config.Prefix + "ns/instance/list?" + r.Encode()
	hc := &http.Client{Timeout: 30 * time.Second}

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Accept", "application/json")
	resp, err := hc.Do(req)

	if err != nil {
		nacosClient.Logger.Errorf("fetch nacos service instance error:%s", uri)
		return nil, errors.New("fetch nacos service instance error")
	}

	nacosResp := model.NacosInstanceResp{}
	err = json.NewDecoder(resp.Body).Decode(&nacosResp)

	_, _ = io.Copy(ioutil.Discard, resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		nacosClient.Logger.Errorf("fetch nacos service instance error:%s", uri)
		return nil, errors.New("fetch nacos service instance error")
	}
	nacosClient.Logger.Debugf("fetch nacos service:%s,instances:%+v", uri, nacosResp.Hosts)
	instances := []model.Instance{}
	for _, host := range nacosResp.Hosts {
		instance := model.Instance{
			Ip:       host.Ip,
			Port:     host.Port,
			Weight:   host.Weight,
			Metadata: host.Metadata,
			Ext: map[string]string{
				"serviceName": host.ServiceName,
				"groupName":   host.GroupName,
				"clusterName": host.ClusterName,
				"namespaceId": host.NamespaceId,
				"ephemeral":   strconv.FormatBool(host.Ephemeral)}}
		for k, v := range vo.ExtData {
			instance.Ext[k] = v
		}
		instances = append(instances, instance)

	}
	return instances, err
}

func (nacosClient *NacosClient) ModifyRegistration(registration model.Registration, instances []model.Instance) error {
	for _, instance := range instances {
		if !instance.Change {
			continue
		}
		//best way is update enabled (doc https://nacos.io/en-us/docs/open-api.html#2.3)
		r := url.Values{}
		for k, v := range instance.Ext {
			r.Set(k, v)
		}
		r.Set("ip", instance.Ip)
		r.Set("port", strconv.Itoa(instance.Port))
		r.Set("weight", fmt.Sprintf("%.2f", instance.Weight))
		r.Set("enabled", strconv.FormatBool(instance.Enabled))
		r.Set("serviceName", registration.ServiceName)
		metadata, err := json.Marshal(instance.Metadata)
		if err != nil {
			nacosClient.Logger.Errorf("convert metadata to json failed,%#v", instance)
			continue
		}
		r.Set("metadata", string(metadata))

		uri := nacosClient.Config.Host + nacosClient.Config.Prefix + "ns/instance?" + r.Encode()
		hc := &http.Client{Timeout: 30 * time.Second}

		req, _ := http.NewRequest("PUT", uri, nil)
		req.Header.Add("Accept", "application/json")
		resp, err := hc.Do(req)
		if err != nil {
			nacosClient.Logger.Errorf("update nacos instance error:%v", instance)
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			nacosClient.Logger.Errorf("update nacos instance error:%v,body:%s", instance, body)
			continue
		}

		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	return nil
}
