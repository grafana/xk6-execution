/*
 *
 * k6 - a next-generation load testing tool
 * Copyright (C) 2021 Load Impact
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package execution

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/dop251/goja"

	"go.k6.io/k6/js/common"
	"go.k6.io/k6/lib"
)

type (
	// RootExecution is the global module instance that will create module
	// instances for each VU.
	RootExecution struct{}
	// Execution is a JS module that returns information about the currently
	// executing test run.
	Execution struct{ *goja.Proxy }
)

// NewModuleInstancePerVU fulfills the k6 modules.HasModuleInstancePerVU
// interface so that each VU will get a separate copy of the module.
func (*RootExecution) NewModuleInstancePerVU() interface{} {
	return &Execution{}
}

// New returns a pointer to a new RootExecution instance.
func New() *RootExecution {
	return &RootExecution{}
}

// WithContext fulfills the k6 modules.HasWithContext interface to allow
// retrieving the VU, scenario and test state from the context used by each VU.
// It initializes a goja.Proxy object for the per-VU module instance, which in
// turn retrieves goja.DynamicObject instances for each property (scenario, vu,
// test).
func (e *Execution) WithContext(getCtx func() context.Context) {
	keys := []string{"scenario", "vu", "test"}

	pcfg := goja.ProxyTrapConfig{
		OwnKeys: func(target *goja.Object) *goja.Object {
			ctx := getCtx()
			rt := common.GetRuntime(ctx)
			return rt.ToValue(keys).ToObject(rt)
		},
		Has: func(target *goja.Object, prop string) (available bool) {
			return sort.SearchStrings(keys, prop) != -1
		},
		Get: func(target *goja.Object, prop string, r goja.Value) goja.Value {
			return dynObjValue(getCtx, target, prop)
		},
		GetOwnPropertyDescriptor: func(target *goja.Object, prop string) (desc goja.PropertyDescriptor) {
			desc.Enumerable, desc.Configurable = goja.FLAG_TRUE, goja.FLAG_TRUE
			desc.Value = dynObjValue(getCtx, target, prop)
			return desc
		},
	}

	ctx := getCtx()
	rt := common.GetRuntime(ctx)
	proxy := rt.NewProxy(rt.NewObject(), &pcfg)
	e.Proxy = &proxy
}

// dynObjValue returns a goja.Value for a specific prop on target.
func dynObjValue(getCtx func() context.Context, target *goja.Object, prop string) goja.Value {
	v := target.Get(prop)
	if v != nil {
		return v
	}

	ctx := getCtx()
	rt := common.GetRuntime(ctx)
	var (
		dobj *execInfo
		err  error
	)
	switch prop {
	case "scenario":
		dobj, err = newScenarioInfo(getCtx)
	case "test":
		dobj, err = newTestInfo(getCtx)
	case "vu":
		dobj, err = newVUInfo(getCtx)
	}

	if err != nil {
		// TODO: Something less drastic?
		common.Throw(rt, err)
	}

	if dobj != nil {
		v = rt.NewDynamicObject(dobj)
	}
	if err := target.Set(prop, v); err != nil {
		common.Throw(rt, err)
	}
	return v
}

// newScenarioInfo returns a goja.DynamicObject implementation to retrieve
// information about the scenario the current VU is running in.
func newScenarioInfo(getCtx func() context.Context) (*execInfo, error) {
	ctx := getCtx()
	vuState := lib.GetState(ctx)
	ss := lib.GetScenarioState(ctx)
	if ss == nil || vuState == nil {
		return nil, errors.New("getting scenario information in the init context is not supported")
	}

	rt := common.GetRuntime(ctx)
	if rt == nil {
		return nil, errors.New("goja runtime is nil in context")
	}

	si := map[string]func() interface{}{
		"name": func() interface{} {
			ctx := getCtx()
			ss := lib.GetScenarioState(ctx)
			return ss.Name
		},
		"executor": func() interface{} {
			ctx := getCtx()
			ss := lib.GetScenarioState(ctx)
			return ss.Executor
		},
		"startTime": func() interface{} { return float64(ss.StartTime.UnixNano()) / 1e9 },
		"progress": func() interface{} {
			p, _ := ss.ProgressFn()
			return p
		},
		"iteration": func() interface{} {
			return vuState.GetScenarioLocalVUIter()
		},
		"iterationGlobal": func() interface{} {
			if vuState.GetScenarioGlobalVUIter != nil {
				return vuState.GetScenarioGlobalVUIter()
			}
			return goja.Null()
		},
	}

	return newExecInfo(rt, si), nil
}

// newTestInfo returns a goja.DynamicObject implementation to retrieve
// information about the overall test run (local instance).
func newTestInfo(getCtx func() context.Context) (*execInfo, error) {
	ctx := getCtx()
	es := lib.GetExecutionState(ctx)
	if es == nil {
		return nil, errors.New("getting test information in the init context is not supported")
	}

	rt := common.GetRuntime(ctx)
	if rt == nil {
		return nil, errors.New("goja runtime is nil in context")
	}

	ti := map[string]func() interface{}{
		"duration": func() interface{} {
			return float64(es.GetCurrentTestRunDuration()) / float64(time.Millisecond)
		},
		"iterationsCompleted": func() interface{} {
			return es.GetFullIterationCount()
		},
		"iterationsInterrupted": func() interface{} {
			return es.GetPartialIterationCount()
		},
		"vusActive": func() interface{} {
			return es.GetCurrentlyActiveVUsCount()
		},
		"vusMax": func() interface{} {
			return es.GetInitializedVUsCount()
		},
	}

	return newExecInfo(rt, ti), nil
}

// newVUInfo returns a goja.DynamicObject implementation to retrieve
// information about the currently executing VU.
func newVUInfo(getCtx func() context.Context) (*execInfo, error) {
	ctx := getCtx()
	vuState := lib.GetState(ctx)
	if vuState == nil {
		return nil, errors.New("getting VU information in the init context is not supported")
	}

	rt := common.GetRuntime(ctx)
	if rt == nil {
		return nil, errors.New("goja runtime is nil in context")
	}

	vi := map[string]func() interface{}{
		"id":        func() interface{} { return vuState.VUID },
		"idGlobal":  func() interface{} { return vuState.VUIDGlobal },
		"iteration": func() interface{} { return vuState.Iteration },
		"iterationScenario": func() interface{} {
			return vuState.GetScenarioVUIter()
		},
	}

	return newExecInfo(rt, vi), nil
}

// execInfo is a goja.DynamicObject implementation to lazily return data only
// on property access.
type execInfo struct {
	rt   *goja.Runtime
	obj  map[string]func() interface{}
	keys []string
}

var _ goja.DynamicObject = &execInfo{}

func newExecInfo(rt *goja.Runtime, obj map[string]func() interface{}) *execInfo {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return &execInfo{obj: obj, keys: keys, rt: rt}
}

func (ei *execInfo) Get(key string) goja.Value {
	if fn, ok := ei.obj[key]; ok {
		return ei.rt.ToValue(fn())
	}
	return goja.Undefined()
}

func (ei *execInfo) Set(key string, val goja.Value) bool { return false }

func (ei *execInfo) Has(key string) bool {
	_, has := ei.obj[key]
	return has
}

func (ei *execInfo) Delete(key string) bool { return false }

func (ei *execInfo) Keys() []string { return ei.keys }
