# xk6-execution

A [k6](https://github.com/grafana/k6) JavaScript extension using the
[xk6](https://github.com/grafana/xk6) system that returns information about a
test run including VU, scenario and test statistics.

| :exclamation: This project is deprecated.<br>The module has [moved to k6 core as `k6/execution`](https://github.com/grafana/k6/releases/tag/v0.34.0). |
|------|


## Build

To build a `k6` binary with this extension, first ensure you have installed the prerequisites:

- [Go toolchain](https://go101.org/article/go-toolchain.html)
- Git

Then:

1. Install `xk6`:
  ```shell
  go install go.k6.io/xk6/cmd/xk6@latest
  ```

2. Build the binary:
  ```shell
  xk6 build master \
    --with github.com/grafana/xk6-execution
  ```


## Example

```javascript
import { sleep } from 'k6';
import exec from 'k6/x/execution';

export let options = {
  scenarios: {
    shared: {
      executor: 'shared-iterations',
      vus: 50,
      iterations: 500,
    }
  }
}

function logObj(msg, o) {
  console.log(msg, JSON.stringify(o, Object.keys(o).sort()));
}

export default function () {
  sleep(1);
  logObj('VU stats:', exec.vu);
  logObj('Scenario stats:', exec.scenario);
  logObj('Test stats:', exec.instance);
}
```

Sample output:

```shell
INFO[0009] VU stats: {"idInInstance":36,"idInTest":36,"iterationInInstance":8,"iterationInScenario":8}  source=console
INFO[0009] Scenario stats: {"executor":"shared-iterations","iterationInInstance":429,"iterationInTest":429,"name":"shared","progress":0.858,"startTime":1624262301.1202478}  source=console
INFO[0009] Test stats: {"currentTestRunDuration":9035.161124,"iterationsCompleted":429,"iterationsInterrupted":0,"vusActive":50,"vusInitialized":50}  source=console
```


TODO:
- [ ] Document all fields
