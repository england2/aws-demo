resource "aws_cloudwatch_metric_alarm" "cpu_spin_high_cpu" {
  alarm_name          = "debian-cpu-spin-high-cpu"
  alarm_description   = "Triggers when debian-cpu-spin average CPU is above 20 percent for one 20-second period"
  comparison_operator = "GreaterThanThreshold"
  # the number of windows/periods that must breach the threshold for an alert to occur
  evaluation_periods = 1
  metric_name        = "CPUUtilization"
  namespace          = "AWS/EC2"
  # period: the size of the metric aggregation window being evaluated
  # here, this means "if in the last 20 seconds..."
  period = 20
  # "... the average CPU utilization is above..."
  statistic = "Average"
  # "... 20%, then trigger an alarm"
  threshold = 20

  dimensions = {
    InstanceId = aws_instance.server.id
  }
}

resource "aws_sqs_queue" "agent_operation_events" {
  name                       = "agent-operation-events"
  message_retention_seconds  = 1209600
  receive_wait_time_seconds  = 20
  visibility_timeout_seconds = 60

  tags = {
    Name = "agent-operation-events"
  }
}

resource "aws_cloudwatch_event_rule" "cpu_spin_alarm_state_change" {
  name        = "cpu-spin-cloudwatch-alarm-state-change"
  description = "Routes debian-cpu-spin CloudWatch alarm state changes to agent-operation."

  event_pattern = jsonencode({
    source        = ["aws.cloudwatch"]
    "detail-type" = ["CloudWatch Alarm State Change"]
    resources     = [aws_cloudwatch_metric_alarm.cpu_spin_high_cpu.arn]
    detail = {
      alarmName = [aws_cloudwatch_metric_alarm.cpu_spin_high_cpu.alarm_name]
      state = {
        value = ["ALARM"]
      }
    }
  })
}

resource "aws_cloudwatch_event_target" "cpu_spin_alarm_to_sqs" {
  rule      = aws_cloudwatch_event_rule.cpu_spin_alarm_state_change.name
  target_id = "agent-operation-events"
  arn       = aws_sqs_queue.agent_operation_events.arn
}

data "aws_iam_policy_document" "agent_operation_events_queue" {
  statement {
    sid    = "AllowEventBridgeToSendAlarmEvents"
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }

    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.agent_operation_events.arn]

    condition {
      test     = "ArnEquals"
      variable = "aws:SourceArn"
      values   = [aws_cloudwatch_event_rule.cpu_spin_alarm_state_change.arn]
    }
  }
}

resource "aws_sqs_queue_policy" "agent_operation_events" {
  queue_url = aws_sqs_queue.agent_operation_events.id
  policy    = data.aws_iam_policy_document.agent_operation_events_queue.json
}

data "aws_iam_policy_document" "agent_operation_sqs_poll" {
  statement {
    sid    = "PollAgentOperationEvents"
    effect = "Allow"

    actions = [
      "sqs:ChangeMessageVisibility",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes",
      "sqs:GetQueueUrl",
      "sqs:ReceiveMessage",
    ]

    resources = [aws_sqs_queue.agent_operation_events.arn]
  }
}

resource "aws_iam_role_policy" "agent_operation_sqs_poll" {
  name   = "${var.agent_operation_instance_name}-sqs-poll"
  role   = aws_iam_role.agent_operation.id
  policy = data.aws_iam_policy_document.agent_operation_sqs_poll.json
}
