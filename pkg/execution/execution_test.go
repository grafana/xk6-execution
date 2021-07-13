package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.k6.io/k6/core/local"
	"go.k6.io/k6/js"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/js/modulestest"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/lib/testutils"
	"go.k6.io/k6/loader"
	"go.k6.io/k6/stats"
)

func TestMain(m *testing.M) {
	modules.Register("k6/x/execution", New())
	os.Exit(m.Run())
}

func TestExecutionInfoVUSharing(t *testing.T) {
	t.Parallel()
	script := []byte(`
		import exec from 'k6/x/execution';
		import { sleep } from 'k6';

		// The cvus scenario should reuse the two VUs created for the carr scenario.
		export let options = {
			scenarios: {
				carr: {
					executor: 'constant-arrival-rate',
					exec: 'carr',
					rate: 9,
					timeUnit: '0.95s',
					duration: '1s',
					preAllocatedVUs: 2,
					maxVUs: 10,
					gracefulStop: '100ms',
				},
			    cvus: {
					executor: 'constant-vus',
					exec: 'cvus',
					vus: 2,
					duration: '1s',
					startTime: '2s',
					gracefulStop: '0s',
			    },
		    },
		};

		export function cvus() {
			const info = Object.assign({scenario: 'cvus'}, exec.vu);
			console.log(JSON.stringify(info));
			sleep(0.2);
		};

		export function carr() {
			const info = Object.assign({scenario: 'carr'}, exec.vu);
			console.log(JSON.stringify(info));
		};
`)

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)
	logHook := testutils.SimpleLogrusHook{HookedLevels: []logrus.Level{logrus.InfoLevel}}
	logger.AddHook(&logHook)

	runner, err := js.New(
		logger,
		&loader.SourceData{
			URL:  &url.URL{Path: "/script.js"},
			Data: script,
		},
		nil,
		lib.RuntimeOptions{},
	)
	require.NoError(t, err)

	ctx, cancel, execScheduler, samples := newTestExecutionScheduler(t, runner, logger, lib.Options{})
	defer cancel()

	type vuStat struct {
		iteration uint64
		scIter    map[string]uint64
	}
	vuStats := map[uint64]*vuStat{}

	type logEntry struct {
		ID, Iteration     uint64
		Scenario          string
		IterationScenario uint64
	}

	errCh := make(chan error, 1)
	go func() { errCh <- execScheduler.Run(ctx, ctx, samples) }()

	select {
	case err := <-errCh:
		require.NoError(t, err)
		entries := logHook.Drain()
		assert.InDelta(t, 20, len(entries), 2)
		le := &logEntry{}
		for _, entry := range entries {
			err = json.Unmarshal([]byte(entry.Message), le)
			require.NoError(t, err)
			assert.Contains(t, []uint64{1, 2}, le.ID)
			if _, ok := vuStats[le.ID]; !ok {
				vuStats[le.ID] = &vuStat{0, make(map[string]uint64)}
			}
			if le.Iteration > vuStats[le.ID].iteration {
				vuStats[le.ID].iteration = le.Iteration
			}
			if le.IterationScenario > vuStats[le.ID].scIter[le.Scenario] {
				vuStats[le.ID].scIter[le.Scenario] = le.IterationScenario
			}
		}
		require.Len(t, vuStats, 2)
		// Both VUs should complete 10 iterations each globally, but 5
		// iterations each per scenario (iterations are 0-based)
		for _, v := range vuStats {
			assert.Equal(t, uint64(9), v.iteration)
			assert.Equal(t, uint64(4), v.scIter["cvus"])
			assert.Equal(t, uint64(4), v.scIter["carr"])
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
}

func TestExecutionInfoScenarioIter(t *testing.T) {
	t.Parallel()
	script := []byte(`
		import exec from 'k6/x/execution';

		// The pvu scenario should reuse the two VUs created for the carr scenario.
		export let options = {
			scenarios: {
				carr: {
					executor: 'constant-arrival-rate',
					exec: 'carr',
					rate: 9,
					timeUnit: '0.95s',
					duration: '1s',
					preAllocatedVUs: 2,
					maxVUs: 10,
					gracefulStop: '100ms',
				},
				pvu: {
					executor: 'per-vu-iterations',
					exec: 'pvu',
					vus: 2,
					iterations: 5,
					startTime: '2s',
					gracefulStop: '100ms',
				},
			},
		};

		export function pvu() {
			const info = Object.assign({VUID: __VU}, exec.scenario);
			console.log(JSON.stringify(info));
		}

		export function carr() {
			const info = Object.assign({VUID: __VU}, exec.scenario);
			console.log(JSON.stringify(info));
		};
`)

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)
	logHook := testutils.SimpleLogrusHook{HookedLevels: []logrus.Level{logrus.InfoLevel}}
	logger.AddHook(&logHook)

	runner, err := js.New(
		logger,
		&loader.SourceData{
			URL:  &url.URL{Path: "/script.js"},
			Data: script,
		},
		nil,
		lib.RuntimeOptions{},
	)
	require.NoError(t, err)

	ctx, cancel, execScheduler, samples := newTestExecutionScheduler(t, runner, logger, lib.Options{})
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- execScheduler.Run(ctx, ctx, samples) }()

	scStats := map[string]uint64{}

	type logEntry struct {
		Name            string
		Iteration, VUID uint64
	}

	select {
	case err := <-errCh:
		require.NoError(t, err)
		entries := logHook.Drain()
		require.Len(t, entries, 20)
		le := &logEntry{}
		for _, entry := range entries {
			err = json.Unmarshal([]byte(entry.Message), le)
			require.NoError(t, err)
			assert.Contains(t, []uint64{1, 2}, le.VUID)
			if le.Iteration > scStats[le.Name] {
				scStats[le.Name] = le.Iteration
			}
		}
		require.Len(t, scStats, 2)
		// The global per scenario iteration count should be 9 (iterations
		// start at 0), despite VUs being shared or more than 1 being used.
		for _, v := range scStats {
			assert.Equal(t, uint64(9), v)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
}

// Ensure that scenario iterations returned from k6/x/execution are
// stable during the execution of an iteration.
func TestSharedIterationsStable(t *testing.T) {
	t.Parallel()
	script := []byte(`
		import { sleep } from 'k6';
		import exec from 'k6/x/execution';

		export let options = {
			scenarios: {
				test: {
					executor: 'shared-iterations',
					vus: 50,
					iterations: 50,
				},
			},
		};
		export default function () {
			sleep(1);
			console.log(JSON.stringify(Object.assign({VUID: __VU}, exec.scenario)));
		}
`)

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)
	logHook := testutils.SimpleLogrusHook{HookedLevels: []logrus.Level{logrus.InfoLevel}}
	logger.AddHook(&logHook)

	runner, err := js.New(
		logger,
		&loader.SourceData{
			URL:  &url.URL{Path: "/script.js"},
			Data: script,
		},
		nil,
		lib.RuntimeOptions{},
	)
	require.NoError(t, err)

	ctx, cancel, execScheduler, samples := newTestExecutionScheduler(t, runner, logger, lib.Options{})
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- execScheduler.Run(ctx, ctx, samples) }()

	expIters := [50]int64{}
	for i := 0; i < 50; i++ {
		expIters[i] = int64(i)
	}
	gotLocalIters, gotGlobalIters := []int64{}, []int64{}

	type logEntry struct{ Iteration, IterationGlobal int64 }

	select {
	case err := <-errCh:
		require.NoError(t, err)
		entries := logHook.Drain()
		require.Len(t, entries, 50)
		le := &logEntry{}
		for _, entry := range entries {
			err = json.Unmarshal([]byte(entry.Message), le)
			require.NoError(t, err)
			require.Equal(t, le.Iteration, le.IterationGlobal)
			gotLocalIters = append(gotLocalIters, le.Iteration)
			gotGlobalIters = append(gotGlobalIters, le.IterationGlobal)
		}

		assert.ElementsMatch(t, expIters, gotLocalIters)
		assert.ElementsMatch(t, expIters, gotGlobalIters)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestExecutionInfo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name, script, expErr string
	}{
		{name: "vu_ok", script: `
		var exec = require('k6/x/execution');

		exports.default = function() {
			if (exec.vu.id !== 1) throw new Error('unexpected VU ID: '+exec.vu.id);
			if (exec.vu.idGlobal !== 10) throw new Error('unexpected global VU ID: '+exec.vu.idGlobal);
			if (exec.vu.iteration !== 0) throw new Error('unexpected VU iteration: '+exec.vu.iteration);
			if (exec.vu.iterationScenario !== 0) throw new Error('unexpected scenario iteration: '+exec.vu.iterationScenario);
		}`},
		{name: "vu_err", script: `
		var exec = require('k6/x/execution');
		exec.vu;
		`, expErr: "getting VU information in the init context is not supported"},
		{name: "scenario_ok", script: `
		var exec = require('k6/x/execution');
		var sleep = require('k6').sleep;

		exports.default = function() {
			var si = exec.scenario;
			sleep(0.1);
			if (si.name !== 'default') throw new Error('unexpected scenario name: '+si.name);
			if (si.executor !== 'test-exec') throw new Error('unexpected executor: '+si.executor);
			if (si.startTime > new Date().getTime()) throw new Error('unexpected startTime: '+si.startTime);
			if (si.progress !== 0.1) throw new Error('unexpected progress: '+si.progress);
			if (si.iteration !== 3) throw new Error('unexpected scenario local iteration: '+si.iteration);
			if (si.iterationGlobal !== 4) throw new Error('unexpected scenario local iteration: '+si.iterationGlobal);
		}`},
		{name: "scenario_err", script: `
		var exec = require('k6/x/execution');
		exec.scenario;
		`, expErr: "getting scenario information in the init context is not supported"},
		{name: "test_ok", script: `
		var exec = require('k6/x/execution');

		exports.default = function() {
			var ti = exec.test;
			if (ti.duration !== 0) throw new Error('unexpected test duration: '+ti.duration);
			if (ti.vusActive !== 1) throw new Error('unexpected vusActive: '+ti.vusActive);
			if (ti.vusMax !== 0) throw new Error('unexpected vusMax: '+ti.vusMax);
			if (ti.iterationsCompleted !== 0) throw new Error('unexpected iterationsCompleted: '+ti.iterationsCompleted);
			if (ti.iterationsInterrupted !== 0) throw new Error('unexpected iterationsInterrupted: '+ti.iterationsInterrupted);
		}`},
		{name: "test_err", script: `
		var exec = require('k6/x/execution');
		exec.test.duration;
		`, expErr: "getting test information in the init context is not supported"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, err := getSimpleRunner(t, "/script.js", tc.script)
			if tc.expErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expErr)
				return
			}
			require.NoError(t, err)

			samples := make(chan stats.SampleContainer, 100)
			initVU, err := r.NewVU(1, 10, samples)
			require.NoError(t, err)

			execScheduler, err := local.NewExecutionScheduler(r, testutils.NewLogger(t))
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ctx = lib.WithExecutionState(ctx, execScheduler.GetState())
			ctx = lib.WithScenarioState(ctx, &lib.ScenarioState{
				Name:      "default",
				Executor:  "test-exec",
				StartTime: time.Now(),
				ProgressFn: func() (float64, []string) {
					return 0.1, nil
				},
			})
			vu := initVU.Activate(&lib.VUActivationParams{
				RunContext:               ctx,
				Exec:                     "default",
				GetNextIterationCounters: func() (uint64, uint64) { return 3, 4 },
			})

			execState := execScheduler.GetState()
			execState.ModCurrentlyActiveVUsCount(+1)
			err = vu.RunOnce()
			assert.NoError(t, err)
		})
	}
}

func TestAbortTest(t *testing.T) { //nolint: tparallel
	t.Parallel()

	rt := goja.New()
	ctx := common.WithRuntime(context.Background(), rt)
	ctx = lib.WithState(ctx, &lib.State{})
	mii := &modulestest.InstanceCore{
		Runtime: rt,
		InitEnv: &common.InitEnvironment{},
		Ctx:     ctx,
	}
	m, ok := New().NewModuleInstance(mii).(*ModuleInstance)
	require.True(t, ok)
	require.NoError(t, rt.Set("exec", m.GetExports().Default))

	prove := func(t *testing.T, script, reason string) {
		_, err := rt.RunString(script)
		require.NotNil(t, err)
		var x *goja.InterruptedError
		assert.ErrorAs(t, err, &x)
		v, ok := x.Value().(*common.InterruptError)
		require.True(t, ok)
		require.Equal(t, v.Reason, reason)
	}

	t.Run("default reason", func(t *testing.T) { //nolint: paralleltest
		prove(t, "exec.test.abort()", common.AbortTest)
	})
	t.Run("custom reason", func(t *testing.T) { //nolint: paralleltest
		prove(t, `exec.test.abort("mayday")`, fmt.Sprintf("%s: mayday", common.AbortTest))
	})
}
