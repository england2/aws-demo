# ticket
<!-- TODO-->
```json

```

# cloudwatch alert

```json
{
    "Messages": [
        {
            "MessageId": "09a7b06c-d7b0-4a88-8dde-feb9f9ba3893",
            "ReceiptHandle": "AQEBJ8zcWXX1ka+qoH4hy0WnZA/Js/o5CS8CD8pX+m/cSYvk+P0ga556m+jIF6p57Gaf1wyzfyDctAlIDJe5ppaGF+Ej4kMEKJyogcc+KMH4GA1wTSB8kIpG1Gnlt1TTAT82vOqHr4/h5PzmjMBWJADgxDKgRqXUl3sc+nxZ/6I5F+v4pjlNlEKfToA8rnoQCmkpr5kCFX6OmU7MX1oCETyoEmBVQwoY6rIZOfUQLJ75Bz83zZpbwX5xkq9tzEYq3pQ0iRxjrR7TQ1zudnRSfRpB43sj0VMCIuJs2enKkxuCelW9JQHe38D0CIfqGzf0ooIRC3jZzL5owi8Zfz5Vr4bOBrc8poUvq/o4noJb5dO4vBXSFMxenP1tgj9bk/CKxvd+PFaFH18lJfeqbhzgMHQZ4w==",
            "MD5OfBody": "ac16c847051e121b1cbe87d5ad9bea75",
            "Body": "{\"version\": \"0\", \"id\": \"4527f63f-b212-495e-b3e7-c1ebf6856f15\", \"detail-type\": \"CloudWatch Alarm State Change\", \"source\": \"aws.cloudwatch\", \"account\": \"204772699175\", \"time\": \"2026-05-12T20:58:58Z\", \"region\": \"us-west-2\", \"resources\": [\"arn:aws:cloudwatch:us-west-2:204772699175:alarm:debian-cpu-spin-high-cpu\"], \"detail\": {\"alarmName\": \"debian-cpu-spin-high-cpu\", \"state\": {\"value\": \"ALARM\", \"reason\": \"Threshold Crossed: 1 datapoint [99.83333333333333 (12/05/26 20:58:58)] was greater than the threshold (20.0).\", \"reasonData\": \"{\\\"version\\\": \\\"1.0\\\", \\\"queryDate\\\": \\\"2026-05-12T20:58:58.130+0000\\\", \\\"startDate\\\": \\\"2026-05-12T20:58:58.000+0000\\\", \\\"statistic\\\": \\\"Average\\\", \\\"period\\\": 20, \\\"recentDatapoints\\\": [99.83333333333333], \\\"threshold\\\": 20.0, \\\"evaluatedDatapoints\\\": [{\\\"timestamp\\\": \\\"2026-05-12T20:58:58.000+0000\\\", \\\"sampleCount\\\": 1.0, \\\"value\\\": 99.83333333333333}]}\", \"timestamp\": \"2026-05-12T20:58:58.130+0000\"}, \"previousState\": {\"value\": \"INSUFFICIENT_DATA\", \"reason\": \"Insufficient Data: 1 datapoint was unknown.\", \"reasonData\": \"{\\\"version\\\": \\\"1.0\\\", \\\"queryDate\\\": \\\"2026-05-12T20:54:58.130+0000\\\", \\\"statistic\\\": \\\"Average\\\", \\\"period\\\": 20, \\\"recentDatapoints\\\": [], \\\"threshold\\\": 20.0, \\\"evaluatedDatapoints\\\": [{\\\"timestamp\\\": \\\"2026-05-12T20:54:58.000+0000\\\"}]}\", \"timestamp\": \"2026-05-12T20:54:58.130+0000\"}, \"configuration\": {\"metrics\": [{\"id\": \"e6ceefb7-f504-ebf4-035d-e13798e92d3f\", \"metricStat\": {\"metric\": {\"namespace\": \"AWS/EC2\", \"name\": \"CPUUtilization\", \"dimensions\": {\"InstanceId\": \"i-03f8306225046aca5\"}}, \"period\": 20, \"stat\": \"Average\"}, \"returnData\": true}], \"description\": \"Triggers when debian-cpu-spin CPU average is above 20 percent for one 20-second period\"}}}"
        }
    ]
}
```
