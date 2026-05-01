from dataclasses import dataclass
from typing import Protocol


class AgentOperationConfig(Protocol):
    operation_name: str

    def run_on_message_pattern(self, message: dict) -> bool:
        """Return true when this operation should respond to the message."""


@dataclass(frozen=True)
class InvestigateCpu:
    # Maps to operations/investigate-cpu/investigate-cpu.py.
    operation_name: str = "investigate-cpu"

    def run_on_message_pattern(self, message: dict) -> bool:
        detail = message.get("detail", {})
        state = detail.get("state", {})

        return (
            message.get("source") == "aws.cloudwatch"
            and message.get("detail-type") == "CloudWatch Alarm State Change"
            and detail.get("alarmName") == "debian-cpu-spin-high-cpu"
            and state.get("value") == "ALARM"
        )


AGENT_OPERATION_CONFIGS: tuple[AgentOperationConfig, ...] = (InvestigateCpu(),)


def matching_agent_operations(message: dict) -> list[AgentOperationConfig]:
    matches = []

    for config in AGENT_OPERATION_CONFIGS:
        if config.run_on_message_pattern(message):
            matches.append(config)

    return matches
