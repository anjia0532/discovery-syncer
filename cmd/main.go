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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/anjia0532/apisix-discovery-syncer/client"
	"github.com/anjia0532/apisix-discovery-syncer/config"
	"github.com/anjia0532/apisix-discovery-syncer/dto"
	"github.com/gorilla/mux"
	"github.com/phachon/go-logger"
	"github.com/robfig/cron/v3"
	"gopkg.in/alecthomas/kingpin.v2"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"
)

var startTime time.Time

func main() {

	startTime = time.Now()
	// pickup command line args
	var (
		listenAddress = kingpin.Flag("web.listen-address",
			"The address to listen on for web interface.").Short('p').Default(":8080").String()
		configFile = kingpin.Flag("config.file",
			"Path to configuration file.").Short('c').Default("config.yml").String()
	)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	flagsMap["web.listen-address"] = *listenAddress
	flagsMap["config.file"] = *configFile

	cfg, err := config.LoadFile(flagsMap["config.file"])

	processed := make(chan struct{})
	r := mux.NewRouter()
	r.Handle("/", http.HandlerFunc(indexHandler))
	r.HandleFunc("/-/reload", reloadHandler)
	r.HandleFunc("/health", healthHandler)
	r.HandleFunc("/discovery/{name}", discoveryHandler)

	if err == nil {
		// default is false
		if cfg.EnablePprof {
			r.PathPrefix("/debug/pprof/").HandlerFunc(pprof.Index)
		}
	}

	srv := http.Server{
		Addr:    flagsMap["web.listen-address"],
		Handler: r,
	}
	go func() {
		_ = run()
		job.Start()
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM)
		<-c

		logger.Flush()
		job.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); nil != err {
			log.Fatalf("server shutdown failed, err: %v\n", err)
		}
		log.Println("server gracefully shutdown")

		close(processed)
	}()
	err = srv.ListenAndServe()
	if http.ErrServerClosed != err {
		log.Fatalf("server not gracefully shutdown, err :%v\n", err)
	}
	<-processed
}

type healthResp struct {
	Total   int      `json:"total"`
	Running int      `json:"running"`
	Lost    int      `json:"lost"`
	Status  string   `json:"status"`
	Details []string `json:"details"`
	Uptime  string   `json:"uptime"`
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	healthMap := client.GetHealthMap()
	healthResp := healthResp{Total: len(syncers), Running: 0, Lost: 0, Status: "OK"}
	for _, syncer := range syncers {
		if time.Now().Unix()-healthMap[syncer.Key] > syncer.MaximumIntervalSec {
			healthResp.Lost += 1
			healthResp.Details = append(healthResp.Details,
				fmt.Sprintf("syncer:%s,Not running for more than %d sec", syncer.Key,
					time.Now().Unix()-healthMap[syncer.Key]))
		} else {
			healthResp.Running += 1
			healthResp.Details = append(healthResp.Details, fmt.Sprintf("syncer:%s,is ok", syncer.Key))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if healthResp.Running == len(syncers) {
		w.WriteHeader(http.StatusOK)
		healthResp.Status = "OK"
	} else if healthResp.Running == 0 && healthResp.Lost > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		healthResp.Status = "DOWN"
	} else if healthResp.Running > 0 && healthResp.Lost > 0 {
		w.WriteHeader(http.StatusOK)
		healthResp.Status = "WARN"
	}
	healthResp.Uptime = fmt.Sprintf("%s", time.Since(startTime).Round(time.Second))
	data, err := json.Marshal(healthResp)
	if err != nil {
		log.Fatal(err)
	}
	_, _ = fmt.Fprintf(w, "%s", data)
}

func discoveryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	discovery, ok := client.GetDiscoveryClient(name)
	if !ok {
		_, _ = fmt.Fprintf(w, "Not Found")
		return
	}

	registration := dto.Registration{}
	err := json.NewDecoder(r.Body).Decode(&registration)
	if err != nil {
		_, _ = fmt.Fprintf(w, err.Error())
	}
	discoveryInstances, err := discovery.GetServiceAllInstances(
		dto.GetInstanceVo{ServiceName: registration.ServiceName, ExtData: registration.ExtData})
	if err != nil {
		_, _ = fmt.Fprintf(w, err.Error())
		return
	}
	instances := []dto.Instance{}
	for _, instance := range discoveryInstances {
		val := ""
		if registration.Type == "METADATA" {
			v, ok := instance.Metadata[registration.MetadataKey]
			val = v
			if !ok {
				if registration.OtherStatus != "ORIGIN" {
					instance.Enabled = registration.OtherStatus == "UP"
					instance.Change = true
				}
				continue
			}
		} else if registration.Type == "IP" {
			val = instance.Ip
		}
		if regexp.MustCompile(registration.RegexpStr).MatchString(val) {
			instance.Enabled = registration.Status == "UP"
			instance.Change = true
		} else {
			if registration.OtherStatus != "ORIGIN" {
				instance.Enabled = registration.OtherStatus == "UP"
				instance.Change = true
			}
		}
		if instance.Change {
			instances = append(instances, instance)
		}
	}
	if len(instances) > 0 {
		err = discovery.ModifyRegistration(registration, instances)
	}
	if err != nil {
		_, _ = fmt.Fprintf(w, err.Error())
	} else {
		_, _ = fmt.Fprintf(w, "OK")
	}
}

func indexHandler(w http.ResponseWriter, _ *http.Request) {
	_, _ = fmt.Fprintf(w, "OK")
}
func reloadHandler(w http.ResponseWriter, _ *http.Request) {
	_ = run()
	_, _ = fmt.Fprintf(w, "OK")
}

var (
	job      = cron.New()
	logger   = go_logger.NewLogger()
	flagsMap = map[string]string{}
	syncers  []client.Syncer
)

func run() int {

	// load config file
	cfg, err := config.LoadFile(flagsMap["config.file"])

	if err != nil {
		logger.Errorf("load configuration error:%s", err)
		return -1
	}

	// reconfiguration logger
	if "file" == cfg.Logger.Logger {
		fileConfig := &go_logger.FileConfig{
			Filename:  cfg.Logger.LogFile,
			DateSlice: cfg.Logger.DateSlice,
		}
		_ = logger.Attach("file", logger.LoggerLevel(cfg.Logger.Level), fileConfig)
	}
	_ = logger.Detach("console")
	_ = logger.Attach("console", logger.LoggerLevel(cfg.Logger.Level), &go_logger.ConsoleConfig{})

	logger.Info("load configuration")

	// get syncers
	syncers, err = client.CreateSyncer(cfg, logger)
	if err != nil {
		return 0
	}
	// for  reload and reconfiguration job
	for _, entry := range job.Entries() {
		job.Remove(entry.ID)
	}
	for _, syncer := range syncers {
		syncer := syncer
		jobId, err := job.AddJob(syncer.FetchInterval,
			// 捕获异常
			cron.NewChain(cron.Recover(cron.DefaultLogger),
				// 如果上一次任务还未完成，则跳过此次执行
				cron.SkipIfStillRunning(cron.DefaultLogger)).Then(&syncer))
		if err != nil {
			return -1
		}
		logger.Infof("job:%s,jobId:%d", syncer.Key, jobId)
	}
	return 0
}
