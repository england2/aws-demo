#!/usr/bin/env python3

import json
import subprocess
import sys
import uuid
from datetime import UTC, datetime, timedelta
from pathlib import Path


def cloudwatch_time(value: datetime) -> str:
    return value.strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "+0000"


def main() -> int:
    event_id = str(uuid.uuid4())
    script_dir = Path(__file__).resolve().parent
    event_time = datetime.now(UTC)
    datapoint_time = event_time.replace(microsecond=0)
    previous_time = event_time - timedelta(minutes=4)
    previous_datapoint_time = previous_time.replace(microsecond=0)

    reason_data = {
        "version": "1.0",
        "queryDate": cloudwatch_time(event_time),
        "startDate": cloudwatch_time(datapoint_time),
        "statistic": "Average",
        "period": 20,
        "recentDatapoints": [99.83333333333333],
        "threshold": 20.0,
        "evaluatedDatapoints": [
            {
                "timestamp": cloudwatch_time(datapoint_time),
                "sampleCount": 1.0,
                "value": 99.83333333333333,
            }
        ],
    }

    previous_reason_data = {
        "version": "1.0",
        "queryDate": cloudwatch_time(previous_time),
        "statistic": "Average",
        "period": 20,
        "recentDatapoints": [],
        "threshold": 20.0,
        "evaluatedDatapoints": [
            {
                "timestamp": cloudwatch_time(previous_datapoint_time),
            }
        ],
    }

    event = {
        "version": "0",
        "id": event_id,
        "detail-type": "CloudWatch Alarm State Change",
        "source": "aws.cloudwatch",
        "account": "204772699175",
        "time": event_time.isoformat(timespec="seconds").replace("+00:00", "Z"),
        "region": "us-west-2",
        "resources": [
            "arn:aws:cloudwatch:us-west-2:204772699175:alarm:debian-cpu-spin-high-cpu"
        ],
        "detail": {
            "alarmName": "debian-cpu-spin-high-cpu",
            "state": {
                "value": "ALARM",
                "reason": f"Threshold Crossed: 1 datapoint [99.83333333333333 ({datapoint_time:%d/%m/%y %H:%M:%S})] was greater than the threshold (20.0).",
                "reasonData": json.dumps(reason_data),
                "timestamp": cloudwatch_time(event_time),
            },
            "previousState": {
                "value": "INSUFFICIENT_DATA",
                "reason": "Insufficient Data: 1 datapoint was unknown.",
                "reasonData": json.dumps(previous_reason_data),
                "timestamp": cloudwatch_time(previous_time),
            },
            "configuration": {
                "metrics": [
                    {
                        "id": "e6ceefb7-f504-ebf4-035d-e13798e92d3f",
                        "metricStat": {
                            "metric": {
                                "namespace": "AWS/EC2",
                                "name": "CPUUtilization",
                                "dimensions": {
                                    "InstanceId": "i-03f8306225046aca5",
                                },
                            },
                            "period": 20,
                            "stat": "Average",
                        },
                        "returnData": True,
                    }
                ],
                "description": "Triggers when debian-cpu-spin CPU average is above 20 percent for one 20-second period",
            },
        },
    }
    text = json.dumps(event)

    subprocess.run(
        [sys.executable, str(script_dir / "push-sqs-message.py"), text],
        check=True,
    )
    print(f"sent faux sqs alert event_id={event_id}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
