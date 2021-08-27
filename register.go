package execution

import (
	"github.com/grafana/xk6-execution/pkg/execution"

	"go.k6.io/k6/js/modules"
)

func init() {
	modules.Register("k6/x/execution", execution.New())
}
