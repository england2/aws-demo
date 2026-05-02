import os
from pathlib import Path

from alert_config import matching_agent_operations
from poll import delete_sqs_message, print_log, wait_for_sqs_alert
from start_agent import start_agent_operation

QUEUE_URL = os.environ.get(
    "AGENT_OPERATION_QUEUE_URL",
    "https://sqs.us-west-2.amazonaws.com/204772699175/agent-operation-events",
)

SEEN_MESSAGES_PATH = Path(
    os.environ.get("AGENT_OPERATION_SEEN_MESSAGES", "/home/admin/seen-messages.txt")
)


def load_seen_message_ids() -> set[str]:
    if not SEEN_MESSAGES_PATH.exists():
        return set()

    return {
        line.strip()
        for line in SEEN_MESSAGES_PATH.read_text().splitlines()
        if line.strip()
    }


def mark_message_seen(event_id: str) -> None:
    SEEN_MESSAGES_PATH.parent.mkdir(parents=True, exist_ok=True)

    with SEEN_MESSAGES_PATH.open("a") as seen_messages:
        seen_messages.write(f"{event_id}\n")


def already_seen(event_id: str) -> bool:
    return event_id in load_seen_message_ids()


def get_instance_id(event: dict) -> str | None:
    metrics = event.get("detail", {}).get("configuration", {}).get("metrics", [])

    for metric_config in metrics:
        dimensions = (
            metric_config.get("metricStat", {}).get("metric", {}).get("dimensions", {})
        )
        instance_id = dimensions.get("InstanceId")

        if instance_id:
            return instance_id

    return None


def handle_message(event: dict) -> None:
    event_id = event["id"]

    if already_seen(event_id):
        print_log(f"already handled event_id={event_id}")
        return

    detail = event.get("detail", {})
    reason = detail.get("state", {}).get("reason")

    print_log(
        "received event "
        f"event_id={event_id} "
        f"source={event.get('source')} "
        f"detail_type={event.get('detail-type')} "
        f"alarm_name={detail.get('alarmName')} "
        f"state={detail.get('state', {}).get('value')} "
        f"previous_state={detail.get('previousState', {}).get('value')} "
        f"instance_id={get_instance_id(event)}"
    )

    matching_operations = matching_agent_operations(event)

    if not matching_operations:
        print_log(f"no matching operation for event_id={event_id}")
        mark_message_seen(event_id)
        return

    for operation in matching_operations:
        print_log(
            "matched operation "
            f"event_id={event_id} "
            f"operation_name={operation.operation_name} "
            f"reason={reason}"
        )
        start_agent_operation(operation.operation_name, alert=event)

    mark_message_seen(event_id)


def consume(queue_url: str) -> None:
    print_log(f"polling sqs queue_url={queue_url}")

    while True:
        event, receipt_handle, message_id = wait_for_sqs_alert(queue_url)

        try:
            handle_message(event)
            delete_sqs_message(queue_url, receipt_handle)
            print_log(f"deleted sqs message_id={message_id}")
        except Exception as exc:
            print_log(f"message handling failed; leaving message for retry: {exc}")


def main() -> None:
    if not QUEUE_URL:
        raise RuntimeError("AGENT_OPERATION_QUEUE_URL is required")

    print_log("agent-operation running...")
    consume(QUEUE_URL)


if __name__ == "__main__":
    # force push
    main()
