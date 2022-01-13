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

package client

import (
	"errors"
	"fmt"
	"github.com/anjia0532/apisix-discovery-syncer/client/discovery"
	"github.com/anjia0532/apisix-discovery-syncer/client/gateway"
	"github.com/anjia0532/apisix-discovery-syncer/model"
	go_logger "github.com/phachon/go-logger"
	"regexp"
	"time"
)

func createDiscoveryClient(discoveryMap map[string]model.Discovery,
	logger *go_logger.Logger) (iClients map[string]discovery.DiscoveryClient, err error) {
	var client discovery.DiscoveryClient
	iClients = make(map[string]discovery.DiscoveryClient)

	for name, server := range discoveryMap {
		switch server.Type {
		case model.EUREKA_DISCOVERY:
			client = &discovery.EurekaClient{Config: server, Logger: logger}
			break
		case model.NACOS_DISCOVERY:
			client = &discovery.NacosClient{Config: server, Logger: logger}
			break
		default:
			return nil, errors.New(fmt.Sprintf("Does not support%s", server.Type))
		}
		iClients[name] = client
	}
	return
}

func createGatewayClient(gatewayMap map[string]model.Gateway,
	logger *go_logger.Logger) (iClients map[string]gateway.GatewayClient, err error) {
	var client gateway.GatewayClient
	iClients = make(map[string]gateway.GatewayClient)

	for name, server := range gatewayMap {
		switch server.Type {
		case model.APISIX_GATEWAY:
			client = &gateway.ApisixClient{Config: server, Logger: logger}
			break
		case model.KONG_GATEWAY:
			client = &gateway.KongClient{Config: server, Logger: logger}
			break
		default:
			return nil, errors.New(fmt.Sprintf("Does not support%s", server.Type))
		}
		iClients[name] = client
	}
	return
}

var (
	discoveryClientMap map[string]discovery.DiscoveryClient
	gatewayClientMap   map[string]gateway.GatewayClient
	healthMap          = make(map[string]int64)
)

func GetDiscoveryClient(name string) (discovery.DiscoveryClient, bool) {
	client, ok := discoveryClientMap[name]
	return client, ok
}

func GetHealthMap() map[string]int64 {
	return healthMap
}

func CreateSyncer(config *model.Config, logger *go_logger.Logger) (syncers []Syncer, err error) {
	discoveryClientMap, err = createDiscoveryClient(config.DiscoveryServers, logger)
	gatewayClientMap, err = createGatewayClient(config.GatewayServers, logger)

	var unid string
	var syncer Syncer
	for _, target := range config.Targets {
		unid = fmt.Sprintf("discovery:%s,gateway:%s", target.Discovery, target.Gateway)
		if !target.Enabled {
			logger.Warningf("%s,not enabled", unid)
			continue
		}
		discoveryClient, ok := discoveryClientMap[target.Discovery]
		if !ok {
			logger.Errorf("%s,discovery not found", unid)
			continue
		}
		gatewayClient, ok := gatewayClientMap[target.Gateway]
		if !ok {
			logger.Errorf("%s,gateway not found", unid)
			continue
		}
		syncer = Syncer{
			DiscoveryClient:    discoveryClient,
			GatewayClient:      gatewayClient,
			FetchInterval:      target.FetchInterval,
			MaximumIntervalSec: target.MaximumIntervalSec,
			Config:             target.Config,
			UpstreamPrefix:     target.UpstreamPrefix,
			ExcludeService:     target.ExcludeService,
			Logger:             logger,
			Key:                target.Name,
		}
		if len(syncer.UpstreamPrefix) == 0 {
			syncer.UpstreamPrefix = target.Discovery
		}
		syncers = append(syncers, syncer)

		healthMap[syncer.Key] = time.Now().Unix()
	}

	return
}

type Syncer struct {
	DiscoveryClient    discovery.DiscoveryClient
	GatewayClient      gateway.GatewayClient
	FetchInterval      string
	Config             map[string]string
	ExcludeService     []string
	Key                string
	Logger             *go_logger.Logger
	UpstreamPrefix     string
	MaximumIntervalSec int64
}

func (syncer *Syncer) Run() {

	services, err := syncer.DiscoveryClient.GetAllService(syncer.Config)
	if err != nil {
		syncer.Logger.Errorf("fetch discovery services failed,syncer:%+v", syncer)
		panic(err)
	}
	var isExclude bool
	for _, service := range services {
		isExclude = false
		for _, name := range syncer.ExcludeService {
			if regexp.MustCompile(name).MatchString(service.Name) {
				isExclude = true
				break
			}
		}
		if isExclude {
			continue
		}
		syncer.syncServiceInstances(service)
	}

	healthMap[syncer.Key] = time.Now().Unix()
	return
}

func (syncer *Syncer) syncServiceInstances(service model.Service) {
	var (
		discoveryInstances []model.Instance
		err                error
	)
	if len(service.Instances) > 0 {
		discoveryInstances = service.Instances
	} else {
		vo := model.GetInstanceVo{ServiceName: service.Name, ExtData: syncer.Config}
		discoveryInstances, err = syncer.DiscoveryClient.GetServiceAllInstances(vo)

		syncer.Logger.Debugf("Sync serviceName:%s", service.Name)
		if err != nil {
			syncer.Logger.Errorf("fetch discovery %s failed,syncer:%+v", service.Name, syncer)
			panic(err)
		}
	}

	gatewayInstances, err := syncer.GatewayClient.GetServiceAllInstances(syncer.getUpstreamName(service.Name))
	if err != nil {
		syncer.Logger.Errorf("fetch gateway %s failed,syncer:%+v", syncer.getUpstreamName(service.Name), syncer)
		panic(err)
	}

	dim := map[string]model.Instance{}
	gim := map[string]model.Instance{}

	for _, instance := range discoveryInstances {
		dim[fmt.Sprintf("%s:%d@%f", instance.Ip, instance.Port, instance.Weight)] = instance
	}
	// 数据不一样的
	for _, instance := range gatewayInstances {
		k := fmt.Sprintf("%s:%d@%f", instance.Ip, instance.Port, instance.Weight)
		if _, ok := dim[k]; ok {
			delete(dim, k)
		} else {
			gim[k] = instance
		}
	}
	if len(gim) == 0 && len(dim) == 0 {
		return
	}

	tdim := map[string]model.Instance{}
	diffIns := []model.Instance{}
	for _, instance := range dim {
		instance.Enabled = true
		tdim[fmt.Sprintf("%s:%d", instance.Ip, instance.Port)] = instance
	}
	for _, instance := range gim {
		k := fmt.Sprintf("%s:%d", instance.Ip, instance.Port)
		// 权重不一致
		if ins, ok := tdim[k]; ok {
			delete(tdim, k)
			ins.Change = true
			// 相同ip端口，权重不一致的，以dim(discoveryInstances)的覆盖gim的
			diffIns = append(diffIns, ins)
		} else {
			// 不存在该ip端口的，删除gim(gatewayInstances)
			instance.Enabled = false
			instance.Change = false
			diffIns = append(diffIns, instance)
		}
	}
	for _, instance := range tdim {
		instance.Change = false
		instance.Enabled = true
		diffIns = append(diffIns, instance)
	}

	if len(diffIns) > 0 {
		tpl, _ := syncer.Config["template"]

		err = syncer.GatewayClient.SyncInstances(syncer.getUpstreamName(service.Name), tpl, discoveryInstances, diffIns)
		if err != nil {
			syncer.Logger.Errorf("update gateway %s failed,discoveryInstances:%s,diffIns:%s,syncer:%+v",
				syncer.getUpstreamName(service.Name), discoveryInstances, diffIns, syncer)
			panic(err)
		}
	}

	syncer.Logger.Infof("Sync serviceName:%s,diffIns:%+v", syncer.getUpstreamName(service.Name), diffIns)
}
func (syncer *Syncer) getUpstreamName(serviceName string) string {
	return syncer.UpstreamPrefix + "-" + serviceName
}
