//  Copyright (c) 2014 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

package cbft

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	bleveHttp "github.com/blevesearch/bleve/http"

	"github.com/gorilla/mux"

	log "github.com/couchbase/clog"
)

var startTime = time.Now()

type RESTMeta struct {
	Path    string
	Method  string
	Handler http.Handler
	Opts    map[string]string
}

func NewManagerRESTRouter(versionMain string, mgr *Manager,
	staticDir, staticETag string, mr *MsgRing) (
	*mux.Router, map[string]RESTMeta, error) {
	// create a router to serve static files
	r := staticFileRouter(staticDir, staticETag, []string{
		"/indexes",
		"/nodes",
		"/monitor",
		"/manage",
		"/logs",
		"/debug",
	})

	meta := map[string]RESTMeta{}
	handle := func(path string, method string, h http.Handler,
		opts map[string]string) {
		meta[path] = RESTMeta{path, method, h, opts}
		r.Handle(path, h).Methods(method)
	}

	handle("/api/index", "GET", NewListIndexHandler(mgr),
		map[string]string{
			"_category":          "Index definition",
			"version introduced": "0.0.0",
		})
	handle("/api/index/{indexName}", "PUT", NewCreateIndexHandler(mgr),
		map[string]string{
			"_category":          "Index definition",
			"version introduced": "0.0.0",
		})
	handle("/api/index/{indexName}", "DELETE", NewDeleteIndexHandler(mgr),
		map[string]string{
			"_category":          "Index definition",
			"version introduced": "0.0.0",
		})
	handle("/api/index/{indexName}", "GET", NewGetIndexHandler(mgr),
		map[string]string{
			"_category":          "Index definition",
			"version introduced": "0.0.0",
		})

	if mgr == nil || mgr.tagsMap == nil || mgr.tagsMap["queryer"] {
		handle("/api/index/{indexName}/count", "GET",
			NewCountHandler(mgr),
			map[string]string{
				"_category":          "Index querying",
				"version introduced": "0.0.0",
			})
		handle("/api/index/{indexName}/query", "POST",
			NewQueryHandler(mgr),
			map[string]string{
				"_category":          "Index querying",
				"version introduced": "0.0.0",
			})
	}

	handle("/api/index/{indexName}/planFreezeControl/{op}", "POST",
		NewIndexControlHandler(mgr, "planFreeze", map[string]bool{
			"freeze":   true,
			"unfreeze": true,
		}),
		map[string]string{
			"_category": "Index management",
			"param: op": `Allowed values for op are
                          "freeze" or "unfreeze".`,
			"version introduced": "0.0.0",
		})
	handle("/api/index/{indexName}/ingestControl/{op}", "POST",
		NewIndexControlHandler(mgr, "write", map[string]bool{
			"pause":  true,
			"resume": true,
		}),
		map[string]string{
			"_category": "Index management",
			"param: op": `Allowed values for op are
                          "pause" or "resume".`,
			"version introduced": "0.0.0",
		})
	handle("/api/index/{indexName}/queryControl/{op}", "POST",
		NewIndexControlHandler(mgr, "read", map[string]bool{
			"allow":    true,
			"disallow": true,
		}),
		map[string]string{
			"_category": "Index management",
			"param: op": `Allowed values for op are
                          "allow" or "disallow".`,
			"version introduced": "0.0.0",
		})

	// We use standard bleveHttp handlers for the /api/pindex-bleve endpoints.
	//
	// TODO: Need to cleanly separate the /api/pindex and
	// /api/pindex-bleve endpoints.
	if mgr == nil || mgr.tagsMap == nil || mgr.tagsMap["pindex"] {
		handle("/api/pindex", "GET",
			NewListPIndexHandler(mgr),
			map[string]string{
				"_category":          "x/(Advanced) Index partition definition",
				"version introduced": "0.0.0",
			})
		handle("/api/pindex/{pindexName}", "GET",
			NewGetPIndexHandler(mgr),
			map[string]string{
				"_category":          "x/(Advanced) Index partition definition",
				"version introduced": "0.0.0",
			})
		handle("/api/pindex/{pindexName}/count", "GET",
			NewCountPIndexHandler(mgr),
			map[string]string{
				"_category":          "x/(Advanced) Index partition querying",
				"version introduced": "0.0.0",
			})
		handle("/api/pindex/{pindexName}/query", "POST",
			NewQueryPIndexHandler(mgr),
			map[string]string{
				"_category":          "x/(Advanced) Index partition querying",
				"version introduced": "0.0.0",
			})

		listIndexesHandler := bleveHttp.NewListIndexesHandler()
		handle("/api/pindex-bleve", "GET", listIndexesHandler,
			map[string]string{
				"_category":          "x/(Advanced) Index partition definition",
				"version introduced": "0.0.0",
			})

		getIndexHandler := bleveHttp.NewGetIndexHandler()
		getIndexHandler.IndexNameLookup = pindexNameLookup
		handle("/api/pindex-bleve/{pindexName}", "GET",
			getIndexHandler,
			map[string]string{
				"_category":          "x/(Advanced) Index partition definition",
				"version introduced": "0.0.0",
			})

		docCountHandler := bleveHttp.NewDocCountHandler("")
		docCountHandler.IndexNameLookup = pindexNameLookup
		handle("/api/pindex-bleve/{pindexName}/count", "GET",
			docCountHandler,
			map[string]string{
				"_category":          "x/(Advanced) Index partition querying",
				"version introduced": "0.0.0",
			})

		searchHandler := bleveHttp.NewSearchHandler("")
		searchHandler.IndexNameLookup = pindexNameLookup
		handle("/api/pindex-bleve/{pindexName}/query", "POST",
			searchHandler,
			map[string]string{
				"_category":          "x/(Advanced) Index partition querying",
				"version introduced": "0.0.0",
			})

		docGetHandler := bleveHttp.NewDocGetHandler("")
		docGetHandler.IndexNameLookup = pindexNameLookup
		docGetHandler.DocIDLookup = docIDLookup
		handle("/api/pindex-bleve/{pindexName}/doc/{docID}", "GET",
			docGetHandler,
			map[string]string{
				"_category":          "y/(Advanced) bleve index diagnostics",
				"version introduced": "0.0.0",
			})

		debugDocHandler := bleveHttp.NewDebugDocumentHandler("")
		debugDocHandler.IndexNameLookup = pindexNameLookup
		debugDocHandler.DocIDLookup = docIDLookup
		handle("/api/pindex-bleve/{pindexName}/docDebug/{docID}", "GET",
			debugDocHandler,
			map[string]string{
				"_category":          "y/(Advanced) bleve index diagnostics",
				"version introduced": "0.0.0",
			})

		listFieldsHandler := bleveHttp.NewListFieldsHandler("")
		listFieldsHandler.IndexNameLookup = pindexNameLookup
		handle("/api/pindex-bleve/{pindexName}/fields", "GET",
			listFieldsHandler,
			map[string]string{
				"_category":          "y/(Advanced) bleve index diagnostics",
				"version introduced": "0.0.0",
			})
	}

	handle("/api/cfg", "GET", NewCfgGetHandler(mgr),
		map[string]string{
			"_category": "Node configuration",
			"_about": `Returns the node's current view
                       of the cluster's configuration.`,
			"version introduced": "0.0.0",
		})
	handle("/api/cfgRefresh", "POST", NewCfgRefreshHandler(mgr),
		map[string]string{
			"_category":          "Node configuration",
			"_about":             `Requests the node to refresh its configuration.`,
			"version introduced": "0.0.0",
		})
	handle("/api/diag", "GET", NewDiagGetHandler(versionMain, mgr, mr),
		map[string]string{
			"_category":          "Node diagnostics",
			"_about":             `Returns large amount of diagnosis information.`,
			"version introduced": "0.0.0",
		})
	handle("/api/log", "GET", NewLogGetHandler(mgr, mr),
		map[string]string{
			"_category":          "Node diagnostics",
			"_about":             `Returns recent log messages and key events for the node.`,
			"version introduced": "0.0.0",
		})
	handle("/api/managerKick", "POST", NewManagerKickHandler(mgr),
		map[string]string{
			"_category": "Node configuration",
			"_about": `Forces the node to replan resource assignments and
                       to update its state to reflect the latest plan.`,
			"version introduced": "0.0.0",
		})
	handle("/api/managerMeta", "GET", NewManagerMetaHandler(mgr),
		map[string]string{
			"_category":          "Node configuration",
			"_about":             `Returns metadata on the node's capabilities.`,
			"version introduced": "0.0.0",
		})

	handle("/api/runtime", "GET",
		NewRuntimeGetHandler(versionMain, mgr),
		map[string]string{
			"_category": "Node diagnostics",
			"_about": `Returns information on the node's software,
                       such as version strings and slow-changing
                       runtime settings.`,
			"version introduced": "0.0.0",
		})

	r.HandleFunc("/api/runtime/args",
		restGetRuntimeArgs).Methods("GET")
	r.HandleFunc("/api/runtime/gc",
		restPostRuntimeGC).Methods("POST")
	r.HandleFunc("/api/runtime/profile/cpu",
		restProfileCPU).Methods("POST")
	r.HandleFunc("/api/runtime/profile/memory",
		restProfileMemory).Methods("POST")
	r.HandleFunc("/api/runtime/stats",
		restGetRuntimeStats).Methods("GET")
	r.HandleFunc("/api/runtime/statsMem",
		restGetRuntimeStatsMem).Methods("GET")

	handle("/api/stats", "GET", NewStatsHandler(mgr),
		map[string]string{
			"_category": "Node monitoring",
			"_about": `Returns current stats metrics, timings and counters
                       for the node.`,
			"version introduced": "0.0.0",
		})

	return r, meta, nil
}

func muxVariableLookup(req *http.Request, name string) string {
	return mux.Vars(req)[name]
}

type RuntimeGetHandler struct {
	versionMain string
	mgr         *Manager
}

func NewRuntimeGetHandler(versionMain string, mgr *Manager) *RuntimeGetHandler {
	return &RuntimeGetHandler{versionMain: versionMain, mgr: mgr}
}

func (h *RuntimeGetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mustEncode(w, map[string]interface{}{
		"versionMain": h.versionMain,
		"versionData": h.mgr.version,
		"arch":        runtime.GOARCH,
		"os":          runtime.GOOS,
		"numCPU":      runtime.NumCPU(),
		"go": map[string]interface{}{
			"GOMAXPROCS": runtime.GOMAXPROCS(0),
			"GOROOT":     runtime.GOROOT(),
			"version":    runtime.Version(),
			"compiler":   runtime.Compiler,
		},
	})
}

func restGetRuntimeArgs(w http.ResponseWriter, r *http.Request) {
	flags := map[string]interface{}{}
	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value
	})

	env := []string(nil)
	for _, e := range os.Environ() {
		if !strings.Contains(e, "PASSWORD") &&
			!strings.Contains(e, "PSWD") &&
			!strings.Contains(e, "AUTH") {
			env = append(env, e)
		}
	}

	groups, groupsErr := os.Getgroups()
	hostname, hostnameErr := os.Hostname()
	user, userErr := user.Current()
	wd, wdErr := os.Getwd()

	mustEncode(w, map[string]interface{}{
		"args":  os.Args,
		"env":   env,
		"flags": flags,
		"process": map[string]interface{}{
			"euid":        os.Geteuid(),
			"gid":         os.Getgid(),
			"groups":      groups,
			"groupsErr":   ErrorToString(groupsErr),
			"hostname":    hostname,
			"hostnameErr": ErrorToString(hostnameErr),
			"pageSize":    os.Getpagesize(),
			"pid":         os.Getpid(),
			"ppid":        os.Getppid(),
			"user":        user,
			"userErr":     ErrorToString(userErr),
			"wd":          wd,
			"wdErr":       ErrorToString(wdErr),
		},
	})
}

func restPostRuntimeGC(w http.ResponseWriter, r *http.Request) {
	runtime.GC()
}

// To start a cpu profiling...
//    curl -X POST http://127.0.0.1:9090/api/runtime/profile/cpu -d secs=5
// To analyze a profiling...
//    go tool pprof ./cbft run-cpu.pprof
func restProfileCPU(w http.ResponseWriter, r *http.Request) {
	secs, err := strconv.Atoi(r.FormValue("secs"))
	if err != nil || secs <= 0 {
		http.Error(w, "incorrect or missing secs parameter", 400)
		return
	}
	fname := "./run-cpu.pprof"
	os.Remove(fname)
	f, err := os.Create(fname)
	if err != nil {
		http.Error(w, fmt.Sprintf("profileCPU:"+
			" couldn't create file: %s, err: %v",
			fname, err), 500)
		return
	}
	log.Printf("profileCPU: start, file: %s", fname)
	err = pprof.StartCPUProfile(f)
	if err != nil {
		http.Error(w, fmt.Sprintf("profileCPU:"+
			" couldn't start CPU profile, file: %s, err: %v",
			fname, err), 500)
		return
	}
	go func() {
		time.Sleep(time.Duration(secs) * time.Second)
		pprof.StopCPUProfile()
		f.Close()
		log.Printf("profileCPU: end, file: %s", fname)
	}()
	w.WriteHeader(204)
}

// To grab a memory profiling...
//    curl -X POST http://127.0.0.1:9090/api/runtime/profile/memory
// To analyze a profiling...
//    go tool pprof ./cbft run-memory.pprof
func restProfileMemory(w http.ResponseWriter, r *http.Request) {
	fname := "./run-memory.pprof"
	os.Remove(fname)
	f, err := os.Create(fname)
	if err != nil {
		http.Error(w, fmt.Sprintf("profileMemory:"+
			" couldn't create file: %v, err: %v",
			fname, err), 500)
		return
	}
	defer f.Close()
	pprof.WriteHeapProfile(f)
}

func restGetRuntimeStatsMem(w http.ResponseWriter, r *http.Request) {
	memStats := &runtime.MemStats{}
	runtime.ReadMemStats(memStats)
	mustEncode(w, memStats)
}

func restGetRuntimeStats(w http.ResponseWriter, r *http.Request) {
	mustEncode(w, map[string]interface{}{
		"startTime": startTime,
		"go": map[string]interface{}{
			"numGoroutine":   runtime.NumGoroutine(),
			"numCgoCall":     runtime.NumCgoCall(),
			"memProfileRate": runtime.MemProfileRate,
		},
	})
}
