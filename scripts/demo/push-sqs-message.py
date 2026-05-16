#!/usr/bin/env python3

import os
import subprocess
import sys


DEFAULT_QUEUE_URL = "https://sqs.us-west-2.amazonaws.com/204772699175/agent-operation-events"
DEFAULT_REGION = "us-west-2"


def main() -> int:
    if len(sys.argv) != 2:
        print('usage: push-sqs-message.py "$message_body"', file=sys.stderr)
        return 1

    message_body = sys.argv[1]
    if not message_body:
        print("message body is empty", file=sys.stderr)
        return 1

    subprocess.run(
        [
            "aws",
            "sqs",
            "send-message",
            "--queue-url",
            os.environ.get("SQS_QUEUE_URL", DEFAULT_QUEUE_URL),
            "--region",
            os.environ.get("AWS_REGION", DEFAULT_REGION),
            "--message-body",
            message_body,
        ],
        check=True,
    )

    return 0

if __name__ == "__main__":
    raise SystemExit(main())

