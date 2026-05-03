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
