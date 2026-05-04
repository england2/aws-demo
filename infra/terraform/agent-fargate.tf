data "aws_iam_policy_document" "ecs_task_assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }

    actions = ["sts:AssumeRole"]
  }
}

resource "aws_ecs_cluster" "agent_fargate" {
  name = "ecs-cluster-agent-fargate"

  tags = {
    Name = "ecs-cluster-agent-fargate"
  }
}

resource "aws_cloudwatch_log_group" "agent_fargate" {
  name              = "/aws/ecs/agent-fargate"
  retention_in_days = 7

  tags = {
    Name = "agent-fargate"
  }
}

resource "aws_security_group" "agent_fargate" {
  name        = "agent-fargate-sg"
  description = "Network access for temporary agent-fargate test tasks"
  vpc_id      = data.aws_vpc.default.id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "agent-fargate-sg"
  }
}

resource "aws_iam_role" "agent_fargate_task" {
  name               = "agent-fargate-task-role"
  assume_role_policy = data.aws_iam_policy_document.ecs_task_assume_role.json

  tags = {
    Name = "agent-fargate-task-role"
  }
}

data "aws_iam_policy_document" "agent_fargate_openai_secret_read" {
  statement {
    sid    = "ReadOpenAIAPIKey"
    effect = "Allow"

    actions = [
      "secretsmanager:GetSecretValue",
    ]

    resources = [
      "arn:aws:secretsmanager:${var.aws_region}:204772699175:secret:openai-key-aws-demo-agent-fargate-*",
    ]
  }
}

resource "aws_iam_role_policy" "agent_fargate_openai_secret_read" {
  name   = "agent-fargate-openai-secret-read"
  role   = aws_iam_role.agent_fargate_task.id
  policy = data.aws_iam_policy_document.agent_fargate_openai_secret_read.json
}

data "aws_iam_policy_document" "agent_fargate_ecs_exec" {
  statement {
    sid    = "AllowECSExecSSMMessages"
    effect = "Allow"

    actions = [
      "ssmmessages:CreateControlChannel",
      "ssmmessages:CreateDataChannel",
      "ssmmessages:OpenControlChannel",
      "ssmmessages:OpenDataChannel",
    ]

    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "agent_fargate_ecs_exec" {
  name   = "agent-fargate-ecs-exec"
  role   = aws_iam_role.agent_fargate_task.id
  policy = data.aws_iam_policy_document.agent_fargate_ecs_exec.json
}

resource "aws_sqs_queue" "agent_fargate_events" {
  name                       = "agent-fargate-events"
  message_retention_seconds  = 1209600
  receive_wait_time_seconds  = 20
  visibility_timeout_seconds = 60

  tags = {
    Name = "agent-fargate-events"
  }
}

data "aws_iam_policy_document" "agent_fargate_event_send" {
  statement {
    sid    = "SendAgentFargateEvents"
    effect = "Allow"

    actions = [
      "sqs:SendMessage",
    ]

    resources = [aws_sqs_queue.agent_fargate_events.arn]
  }
}

resource "aws_iam_role_policy" "agent_fargate_event_send" {
  name   = "agent-fargate-event-send"
  role   = aws_iam_role.agent_fargate_task.id
  policy = data.aws_iam_policy_document.agent_fargate_event_send.json
}

resource "aws_iam_role" "agent_fargate_execution" {
  name               = "agent-fargate-execution-role"
  assume_role_policy = data.aws_iam_policy_document.ecs_task_assume_role.json

  tags = {
    Name = "agent-fargate-execution-role"
  }
}

resource "aws_iam_role_policy_attachment" "agent_fargate_execution" {
  role       = aws_iam_role.agent_fargate_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_ecs_task_definition" "agent_fargate" {
  family                   = "agent-fargate"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "2048"
  memory                   = "6144"
  task_role_arn            = aws_iam_role.agent_fargate_task.arn
  execution_role_arn       = aws_iam_role.agent_fargate_execution.arn

  runtime_platform {
    cpu_architecture        = "ARM64"
    operating_system_family = "LINUX"
  }

  container_definitions = jsonencode([
    {
      name      = "agent-fargate"
      image     = "${aws_ecr_repository.agent_fargate.repository_url}:latest"
      essential = true

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.agent_fargate.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "agent-fargate"
        }
      }
    }
  ])

  tags = {
    Name = "agent-fargate"
  }
}

data "aws_iam_policy_document" "agent_operation_fargate_control" {
  statement {
    sid    = "RunAgentFargateTask"
    effect = "Allow"

    actions = [
      "ecs:RunTask",
    ]

    resources = [aws_ecs_task_definition.agent_fargate.arn]
  }

  statement {
    sid    = "MonitorAgentFargateTasks"
    effect = "Allow"

    actions = [
      "ecs:DescribeTasks",
      "ecs:ListTasks",
      "ecs:StopTask",
    ]

    resources = ["*"]
  }

  statement {
    sid    = "PassAgentFargateRoles"
    effect = "Allow"

    actions = [
      "iam:PassRole",
    ]

    resources = [
      aws_iam_role.agent_fargate_task.arn,
      aws_iam_role.agent_fargate_execution.arn,
    ]
  }

  statement {
    sid    = "PollAgentFargateEvents"
    effect = "Allow"

    actions = [
      "sqs:ChangeMessageVisibility",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes",
      "sqs:GetQueueUrl",
      "sqs:ReceiveMessage",
    ]

    resources = [aws_sqs_queue.agent_fargate_events.arn]
  }
}

resource "aws_iam_role_policy" "agent_operation_fargate_control" {
  name   = "${var.agent_operation_instance_name}-fargate-control"
  role   = aws_iam_role.agent_operation.id
  policy = data.aws_iam_policy_document.agent_operation_fargate_control.json
}
