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
	"github.com/anjia0532/apisix-discovery-syncer/config"
	"github.com/anjia0532/apisix-discovery-syncer/dto"
	go_logger "github.com/phachon/go-logger"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
)

type EurekaClient struct {
	Client http.Client
	Config config.Discovery
	Logger *go_logger.Logger
}

var HostPageRE = regexp.MustCompile(`^https?://(?P<Ip>[\w.]+):(?P<Port>\d+)/?$`)

func (eurekaClient *EurekaClient) GetServiceAllInstances(vo dto.GetInstanceVo) ([]dto.Instance, error) {
	uri := eurekaClient.Config.Host + eurekaClient.Config.Prefix + "apps/" + vo.ServiceName
	hc := &http.Client{}

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Accept", "application/json")
	resp, err := hc.Do(req)

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if err != nil {
		return nil, err
	}
	if 404 == resp.StatusCode {
		return []dto.Instance{}, nil
	} else if 200 != resp.StatusCode {
		eurekaClient.Logger.Errorf("fetch eureka service error:%s", uri)
		return nil, errors.New("fetch eureka service error")
	}
	eurekaResp := EurekaAppResp{}
	err = json.NewDecoder(resp.Body).Decode(&eurekaResp)

	if err != nil {
		return nil, err
	}

	instances := []dto.Instance{}

	for _, eurekaIns := range eurekaResp.Application.Instance {
		if "UP" != eurekaIns.Status {
			continue
		}
		matches := HostPageRE.FindStringSubmatch(eurekaIns.HomePageUrl)

		port, _ := strconv.Atoi(matches[HostPageRE.SubexpIndex("Port")])
		instance := dto.Instance{Ip: matches[HostPageRE.SubexpIndex("Ip")], Port: port,
			Metadata: eurekaIns.Metadata, Weight: eurekaClient.Config.Weight,
			Ext: map[string]string{"instanceId": eurekaIns.InstanceId}}

		instances = append(instances, instance)
	}
	eurekaClient.Logger.Debugf("fetch eureka service:%s,instances:%+v", uri, instances)
	return instances, nil
}

func (eurekaClient *EurekaClient) ModifyRegistration(registration dto.Registration, instances []dto.Instance) error {
	for _, instance := range instances {
		if !instance.Change {
			continue
		}
		status := "UP"
		if !instance.Enabled {
			status = "OUT_OF_SERVICE"
		}
		// OUT_OF_SERVICE enabled is false
		// UP enabled is true
		// PUT /eureka/v2/apps/appID/instanceID/status?value=OUT_OF_SERVICE
		uri := eurekaClient.Config.Host + eurekaClient.Config.Prefix + "apps/" + registration.ServiceName + "/" +
			instance.Ext["instanceId"] + "/status/?value=" + status
		hc := &http.Client{}

		req, _ := http.NewRequest("PUT", uri, nil)
		req.Header.Add("Accept", "application/json")
		resp, err := hc.Do(req)
		if err != nil {
			eurekaClient.Logger.Errorf("change eureka %s instance %#v failed", registration.ServiceName,
				instance)
			return err
		}
		body, _ := ioutil.ReadAll(resp.Body)
		eurekaClient.Logger.Debugf("change eureka %s instance %#v,body:%s,status:%s", registration.ServiceName,
			instance, string(body), resp.Status)
		_ = resp.Body.Close()
	}
	return nil
}

type EurekaAppResp struct {
	Application EurekaApp `json:"application"`
}
type EurekaApp struct {
	Instance []EurekaInstance `json:"instance"`
}
type EurekaInstance struct {
	HomePageUrl string            `json:"homePageUrl"`
	Status      string            `json:"status"`
	Metadata    map[string]string `json:"metadata"`
	InstanceId  string            `json:"instanceId"`
}
