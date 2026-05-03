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
