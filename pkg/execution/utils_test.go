package execution

import (
	"context"
	"net/url"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v3"

	"go.k6.io/k6/core/local"
	"go.k6.io/k6/js"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/lib/executor"
	"go.k6.io/k6/lib/testutils"
	"go.k6.io/k6/lib/testutils/minirunner"
	"go.k6.io/k6/loader"
	"go.k6.io/k6/stats"
)

// Copied from k6:js/console_test.go
func getSimpleRunner(tb testing.TB, filename, data string, opts ...interface{}) (lib.Runner, error) {
	var (
		fs     = afero.NewMemMapFs()
		rtOpts = lib.RuntimeOptions{CompatibilityMode: null.NewString("base", true)}
		logger = testutils.NewLogger(tb)
	)
	for _, o := range opts {
		switch opt := o.(type) {
		case afero.Fs:
			fs = opt
		case lib.RuntimeOptions:
			rtOpts = opt
		case *logrus.Logger:
			logger = opt
		}
	}
	return js.New(
		logger,
		&loader.SourceData{
			URL:  &url.URL{Path: filename, Scheme: "file"},
			Data: []byte(data),
		},
		map[string]afero.Fs{"file": fs, "https": afero.NewMemMapFs()},
		rtOpts,
	)
}

// Copied from k6:core/local/local_test.go
func newTestExecutionScheduler(
	t *testing.T, runner lib.Runner, logger *logrus.Logger, opts lib.Options,
) (ctx context.Context, cancel func(), execScheduler *local.ExecutionScheduler, samples chan stats.SampleContainer) {
	if runner == nil {
		runner = &minirunner.MiniRunner{}
	}
	ctx, cancel = context.WithCancel(context.Background())
	newOpts, err := executor.DeriveScenariosFromShortcuts(lib.Options{
		MetricSamplesBufferSize: null.NewInt(200, false),
	}.Apply(runner.GetOptions()).Apply(opts))
	require.NoError(t, err)
	require.Empty(t, newOpts.Validate())

	require.NoError(t, runner.SetOptions(newOpts))

	if logger == nil {
		logger = logrus.New()
		logger.SetOutput(testutils.NewTestOutput(t))
	}

	execScheduler, err = local.NewExecutionScheduler(runner, logger)
	require.NoError(t, err)

	samples = make(chan stats.SampleContainer, newOpts.MetricSamplesBufferSize.Int64)
	go func() {
		for {
			select {
			case <-samples:
			case <-ctx.Done():
				return
			}
		}
	}()

	require.NoError(t, execScheduler.Init(ctx, samples))

	return ctx, cancel, execScheduler, samples
}
