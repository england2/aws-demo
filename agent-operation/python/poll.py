import json
import os
import time

import boto3
from botocore.exceptions import ClientError


AWS_REGION = os.environ.get("AWS_REGION", "us-west-2")
VISIBILITY_TIMEOUT_SECONDS = int(
    os.environ.get("AGENT_OPERATION_VISIBILITY_TIMEOUT_SECONDS", "60")
)
WAIT_TIME_SECONDS = int(os.environ.get("AGENT_OPERATION_WAIT_TIME_SECONDS", "20"))


sqs = boto3.client("sqs", region_name=AWS_REGION)


def print_log(message: str) -> None:
    print(message, flush=True)


def parse_sqs_body(message: dict) -> dict:
    return json.loads(message["Body"])


def wait_for_sqs_alert(queue_url: str) -> tuple[dict, str, str]:
    while True:
        try:
            response = sqs.receive_message(
                QueueUrl=queue_url,
                MaxNumberOfMessages=1,
                WaitTimeSeconds=WAIT_TIME_SECONDS,
                VisibilityTimeout=VISIBILITY_TIMEOUT_SECONDS,
            )
        except ClientError as exc:
            print_log(f"sqs receive failed: {exc}")
            time.sleep(5)
            continue

        messages = response.get("Messages", [])

        if not messages:
            continue

        message = messages[0]
        event = parse_sqs_body(message)
        return event, message["ReceiptHandle"], message.get("MessageId", "")


def delete_sqs_message(queue_url: str, receipt_handle: str) -> None:
    sqs.delete_message(
        QueueUrl=queue_url,
        ReceiptHandle=receipt_handle,
    )
