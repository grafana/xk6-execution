import { sleep } from 'k6';
import exec from 'k6/x/execution';

export let options = {
  scenarios: {
    shared: {
      executor: 'shared-iterations',
      vus: 50,
      iterations: 500,
    },
    rar: {
      executor: 'ramping-arrival-rate',
      startTime: '10s',
      startRate: 20,
      timeUnit: '1s',
      preAllocatedVUs: 0,
      maxVUs: 40,
      stages: [
        { target: 50, duration: '5s' },
        { target: 0, duration: '5s' },
      ],
      gracefulStop: '0s',
    },
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
