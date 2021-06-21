# xk6-execution

A [k6](https://github.com/k6io/k6) JavaScript extension using the
[xk6](https://github.com/k6io/xk6) system that returns information about a
test run including VU, scenario and test statistics.

| :exclamation: This project is in active development and may break in the future.<br>Please report issues and contribute improvements to help. |
|------|


## Build

To build a `k6` binary with this extension, first ensure you have installed the prerequisites:

- [Go toolchain](https://go101.org/article/go-toolchain.html)
- Git

Then:

1. Install `xk6`:
  ```shell
  go install github.com/k6io/xk6/cmd/xk6@latest
  ```

2. Build the binary:
  ```shell
  xk6 build master \
    --with github.com/k6io/xk6-execution
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
  const vuStats = exec.getVUStats();
  const scStats = exec.getScenarioStats();
  const testStats = exec.getTestInstanceStats();
  sleep(1);
  logObj('VU stats:', vuStats);
  logObj('Scenario stats:', scStats);
  logObj('Test stats:', testStats);
}
```

Sample output:

```shell
INFO[0009] VU stats: {"id":36,"idGlobal":36,"iteration":8,"iterationScenario":8}  source=console
INFO[0009] Scenario stats: {"executor":"shared-iterations","iteration":429,"iterationGlobal":429,"name":"shared","progress":0.858,"startTime":1624262301.1202478}  source=console
INFO[0009] Test stats: {"duration":9035.161124,"iterationsCompleted":429,"iterationsInterrupted":0,"vusActive":50,"vusMax":50}  source=console
```


TODO:
- [ ] Document all fields
