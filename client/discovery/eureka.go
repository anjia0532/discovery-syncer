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
	"github.com/anjia0532/apisix-discovery-syncer/model"
	go_logger "github.com/phachon/go-logger"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

type EurekaClient struct {
	Client http.Client
	Config model.Discovery
	Logger *go_logger.Logger
}

var HostPageRE = regexp.MustCompile(`^https?://(?P<Ip>[\w.]+):(?P<Port>\d+)/?$`)

func (eurekaClient *EurekaClient) GetAllService(map[string]string) ([]model.Service, error) {
	uri := eurekaClient.Config.Host + eurekaClient.Config.Prefix + "apps/"
	hc := &http.Client{Timeout: 30 * time.Second}

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Accept", "application/json")
	resp, err := hc.Do(req)

	if err != nil {
		return nil, err
	}
	if 404 == resp.StatusCode {
		return []model.Service{}, nil
	} else if 200 != resp.StatusCode {
		eurekaClient.Logger.Errorf("fetch eureka service error, uri:%s, err:%s", uri, err)
		return nil, errors.New("fetch eureka service error")
	}

	eurekaResp := model.EurekaAppsResp{}
	err = json.NewDecoder(resp.Body).Decode(&eurekaResp)
	_, _ = io.Copy(ioutil.Discard, resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	services := []model.Service{}
	for _, app := range eurekaResp.Applications.Application {

		services = append(services, model.Service{Name: app.Name,
			Instances: convertEurekaInstance(app.Instance, eurekaClient.Config.Weight)})
	}
	return services, nil
}

func (eurekaClient *EurekaClient) GetServiceAllInstances(vo model.GetInstanceVo) ([]model.Instance, error) {
	uri := eurekaClient.Config.Host + eurekaClient.Config.Prefix + "apps/" + vo.ServiceName
	hc := &http.Client{Timeout: 30 * time.Second}

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Accept", "application/json")
	resp, err := hc.Do(req)

	if err != nil {
		return nil, err
	}
	if 404 == resp.StatusCode {
		return []model.Instance{}, nil
	} else if 200 != resp.StatusCode {
		eurekaClient.Logger.Errorf("fetch eureka service instance error, uri:%s, err:%s", uri, err)
		return nil, errors.New("fetch eureka service instance error")
	}
	eurekaResp := model.EurekaAppResp{}
	err = json.NewDecoder(resp.Body).Decode(&eurekaResp)

	eurekaClient.Logger.Debugf("fetch eureka service,uri:%s,%#v", uri, eurekaResp)

	_, _ = io.Copy(ioutil.Discard, resp.Body)
	_ = resp.Body.Close()

	if err != nil {
		return nil, err
	}

	instances := convertEurekaInstance(eurekaResp.Application.Instance, eurekaClient.Config.Weight)
	eurekaClient.Logger.Debugf("fetch eureka service:%s,instances:%#v", uri, instances)
	return instances, nil
}

func convertEurekaInstance(eurekaApps []model.EurekaInstance, defaultWeight float32) []model.Instance {

	instances := []model.Instance{}
	if len(eurekaApps) == 0 {
		return instances
	}
	for _, eurekaIns := range eurekaApps {
		if "UP" != eurekaIns.Status {
			continue
		}
		matches := HostPageRE.FindStringSubmatch(eurekaIns.HomePageUrl)

		port, _ := strconv.Atoi(matches[HostPageRE.SubexpIndex("Port")])
		instance := model.Instance{Ip: matches[HostPageRE.SubexpIndex("Ip")], Port: port,
			Metadata: eurekaIns.Metadata, Weight: defaultWeight,
			Ext: map[string]string{"instanceId": eurekaIns.InstanceId}}

		instances = append(instances, instance)
	}
	return instances
}
func (eurekaClient *EurekaClient) ModifyRegistration(registration model.Registration, instances []model.Instance) error {
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
		hc := &http.Client{Timeout: 30 * time.Second}

		req, _ := http.NewRequest("PUT", uri, nil)
		req.Header.Add("Accept", "application/json")
		resp, err := hc.Do(req)
		if err != nil {
			eurekaClient.Logger.Errorf("change eureka %s instance %#v failed, err:%s", registration.ServiceName,
				instance, err)
			return err
		}
		body, _ := ioutil.ReadAll(resp.Body)
		eurekaClient.Logger.Debugf("change eureka %s instance %#v,body:%s,status:%s", registration.ServiceName,
			instance, string(body), resp.Status)
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	return nil
}
