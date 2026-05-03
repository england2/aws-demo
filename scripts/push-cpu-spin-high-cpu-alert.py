#!/usr/bin/env python3

import json
import subprocess
import sys
import uuid
from pathlib import Path


def main() -> int:
    event_id = str(uuid.uuid4())
    script_dir = Path(__file__).resolve().parent

    reason_data = """
{
  "version": "1.0",
  "queryDate": "2026-05-01T04:15:44.090+0000",
  "startDate": "2026-05-01T04:15:00.000+0000",
  "statistic": "Average",
  "period": 20,
  "recentDatapoints": [99.83333333333333],
  "threshold": 20.0,
  "evaluatedDatapoints": [
    {
      "timestamp": "2026-05-01T04:15:00.000+0000",
      "sampleCount": 1.0,
      "value": 99.83333333333333
    }
  ]
}
""".strip()

    previous_reason_data = """
{
  "version": "1.0",
  "queryDate": "2026-05-01T04:11:54.091+0000",
  "statistic": "Average",
  "period": 20,
  "recentDatapoints": [],
  "threshold": 20.0,
  "evaluatedDatapoints": [
    {
      "timestamp": "2026-05-01T04:11:20.000+0000"
    }
  ]
}
""".strip()

    text = f"""
{{
  "version": "0",
  "id": "{event_id}",
  "detail-type": "CloudWatch Alarm State Change",
  "source": "aws.cloudwatch",
  "account": "204772699175",
  "time": "2026-05-01T04:15:44Z",
  "region": "us-west-2",
  "resources": [
    "arn:aws:cloudwatch:us-west-2:204772699175:alarm:debian-cpu-spin-high-cpu"
  ],
  "detail": {{
    "alarmName": "debian-cpu-spin-high-cpu",
    "state": {{
      "value": "ALARM",
      "reason": "Threshold Crossed: 1 datapoint [99.83333333333333 (01/05/26 04:15:00)] was greater than the threshold (20.0).",
      "reasonData": {json.dumps(reason_data)},
      "timestamp": "2026-05-01T04:15:44.091+0000"
    }},
    "previousState": {{
      "value": "INSUFFICIENT_DATA",
      "reason": "Insufficient Data: 1 datapoint was unknown.",
      "reasonData": {json.dumps(previous_reason_data)},
      "timestamp": "2026-05-01T04:11:54.092+0000"
    }},
    "configuration": {{
      "metrics": [
        {{
          "id": "e6ceefb7-f504-ebf4-035d-e13798e92d3f",
          "metricStat": {{
            "metric": {{
              "namespace": "AWS/EC2",
              "name": "CPUUtilization",
              "dimensions": {{
                "InstanceId": "i-03f8306225046aca5"
              }}
            }},
            "period": 20,
            "stat": "Average"
          }},
          "returnData": true
        }}
      ],
      "description": "Triggers when debian-cpu-spin CPU average is above 20 percent for one 20-second period"
    }}
  }}
}}
""".strip()

    subprocess.run(
        [sys.executable, str(script_dir / "push-sqs-message.py"), text],
        check=True,
    )
    print(f"sent faux sqs alert event_id={event_id}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
