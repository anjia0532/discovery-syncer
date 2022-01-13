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
	"github.com/anjia0532/apisix-discovery-syncer/model"
)

type DiscoveryClient interface {
	GetAllService(data map[string]string) ([]model.Service, error)

	// GetServiceAllInstances get instances by serviceName
	GetServiceAllInstances(vo model.GetInstanceVo) ([]model.Instance, error)

	// ModifyRegistration modify discovery registration info by body json
	ModifyRegistration(registration model.Registration, instances []model.Instance) error
}
