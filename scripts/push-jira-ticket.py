#!/usr/bin/env python3

import json
import subprocess
import sys
from datetime import UTC, datetime
from pathlib import Path


def jira_time(value: datetime) -> str:
    return value.strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "+0000"


def update_jira_dates(value, timestamp: str) -> None:
    if isinstance(value, dict):
        for key, nested_value in value.items():
            if key in {"created", "updated"}:
                value[key] = timestamp
            else:
                update_jira_dates(nested_value, timestamp)
    elif isinstance(value, list):
        for item in value:
            update_jira_dates(item, timestamp)


def build_message(script_dir: Path, ticket_text: str) -> str:
    template_path = script_dir / "message-templates" / "jira.json"
    issue = json.loads(template_path.read_text())
    fields = issue["fields"]
    fields["summary"] = ticket_text.splitlines()[0][:120]
    fields["description"]["content"][0]["content"][0]["text"] = ticket_text

    now = jira_time(datetime.now(UTC))
    fields["created"] = now
    fields["updated"] = now
    update_jira_dates(fields["comment"], now)

    return json.dumps(issue)


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: push-jira-ticket.py <ticket text>", file=sys.stderr)
        return 2

    script_dir = Path(__file__).resolve().parent
    text = build_message(script_dir, sys.argv[1])

    subprocess.run(
        [sys.executable, str(script_dir / "push-sqs-message.py"), text],
        check=True,
    )

    print("sent faux jira ticket")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
