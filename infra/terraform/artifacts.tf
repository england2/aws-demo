resource "aws_s3_bucket" "agent_operation_artifacts" {
  bucket = var.agent_operation_artifact_bucket_name

  tags = {
    Name = var.agent_operation_artifact_bucket_name
  }
}

resource "aws_s3_bucket_public_access_block" "agent_operation_artifacts" {
  bucket = aws_s3_bucket.agent_operation_artifacts.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "agent_operation_artifacts" {
  bucket = aws_s3_bucket.agent_operation_artifacts.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_versioning" "agent_operation_artifacts" {
  bucket = aws_s3_bucket.agent_operation_artifacts.id

  versioning_configuration {
    status = "Enabled"
  }
}

data "aws_iam_policy_document" "github_actions_agent_operation_artifact_write" {
  statement {
    sid    = "WriteAgentOperationArtifacts"
    effect = "Allow"
    actions = [
      "s3:PutObject",
    ]
    resources = ["${aws_s3_bucket.agent_operation_artifacts.arn}/*"]
  }
}

resource "aws_iam_policy" "github_actions_agent_operation_artifact_write" {
  name   = "github-actions-agent-operation-artifact-write"
  policy = data.aws_iam_policy_document.github_actions_agent_operation_artifact_write.json
}

resource "aws_iam_role_policy_attachment" "github_actions_agent_operation_artifact_write" {
  role       = "github-actions-ecr-push"
  policy_arn = aws_iam_policy.github_actions_agent_operation_artifact_write.arn
}
