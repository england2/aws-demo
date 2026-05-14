# ticket

```json
{
  "id": "10002",
  "key": "ENG-123",
  "self": "https://your-domain.atlassian.net/rest/api/3/issue/10002",
  "fields": {
    "summary": "Login fails after session timeout",
    "description": {
      "type": "doc",
      "version": 1,
      "content": [
        {
          "type": "paragraph",
          "content": [
            {
              "type": "text",
              "text": "Users are redirected to a blank page after their session expires."
            }
          ]
        }
      ]
    },
    "issuetype": {
      "id": "10004",
      "name": "Bug",
      "subtask": false
    },
    "project": {
      "id": "10000",
      "key": "ENG",
      "name": "Engineering"
    },
    "status": {
      "id": "10001",
      "name": "To Do",
      "statusCategory": {
        "id": 2,
        "key": "new",
        "name": "To Do"
      }
    },
    "priority": {
      "id": "2",
      "name": "High"
    },
    "assignee": {
      "accountId": "abc123",
      "displayName": "Jane Developer",
      "active": true
    },
    "reporter": {
      "accountId": "def456",
      "displayName": "Sam Reporter",
      "active": true
    },
    "created": "2026-05-12T18:22:14.000+0000",
    "updated": "2026-05-13T09:41:03.000+0000",
    "labels": ["auth", "session"],
    "components": [
      {
        "id": "10010",
        "name": "Authentication"
      }
    ],
    "fixVersions": [],
    "attachment": [
      {
        "id": "10000",
        "filename": "screenshot.png",
        "mimeType": "image/png",
        "size": 23123,
        "content": "https://your-domain.atlassian.net/rest/api/3/attachment/content/10000"
      }
    ],
    "comment": {
      "comments": [
        {
          "id": "10001",
          "body": {
            "type": "doc",
            "version": 1,
            "content": [
              {
                "type": "paragraph",
                "content": [
                  {
                    "type": "text",
                    "text": "This reproduces in Chrome and Firefox."
                  }
                ]
              }
            ]
          },
          "author": {
            "accountId": "def456",
            "displayName": "Sam Reporter"
          },
          "created": "2026-05-12T19:00:00.000+0000",
          "updated": "2026-05-12T19:00:00.000+0000"
        }
      ],
      "maxResults": 1,
      "total": 1,
      "startAt": 0
    },
    "issuelinks": [
      {
        "id": "10020",
        "type": {
          "id": "10000",
          "name": "Blocks",
          "inward": "is blocked by",
          "outward": "blocks"
        },
        "outwardIssue": {
          "id": "10003",
          "key": "ENG-124",
          "fields": {
            "summary": "Session refresh endpoint returns 500",
            "status": {
              "name": "In Progress"
            }
          }
        }
      }
    ],
    "subtasks": [],
    "customfield_10011": "Sprint 12",
    "customfield_10014": "ENG-99"
  }
}
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
