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
	"github.com/anjia0532/apisix-discovery-syncer/model"
)

type GatewayClient interface {
	GetServiceAllInstances(upstreamName string) ([]model.Instance, error)

	SyncInstances(name string, tpl string, discoveryInstances []model.Instance, diffIns []model.Instance) error

	FetchAdminApiToFile() (string, string, error)

	MigrateTo(gateway GatewayClient) error
}
