resource "aws_security_group" "agent_operation" {
  name        = "${var.agent_operation_instance_name}-sg"
  description = "SSH access for ${var.agent_operation_instance_name}"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ssh_allowed_cidr]
  }

  ingress {
    description     = "Conductor gRPC from agent Fargate workers"
    from_port       = 50055
    to_port         = 50055
    protocol        = "tcp"
    security_groups = [aws_security_group.agent_fargate.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.agent_operation_instance_name}-sg"
  }
}

resource "aws_iam_role" "agent_operation" {
  name               = "${var.agent_operation_instance_name}-role"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume_role.json

  tags = {
    Name = "${var.agent_operation_instance_name}-role"
  }
}

resource "aws_iam_role_policy" "agent_operation_ecr_pull" {
  name   = "${var.agent_operation_instance_name}-ecr-pull"
  role   = aws_iam_role.agent_operation.id
  policy = data.aws_iam_policy_document.ecr_pull.json
}

data "aws_iam_policy_document" "agent_operation_artifact_read" {
  statement {
    sid    = "ReadAgentOperationArtifacts"
    effect = "Allow"
    actions = [
      "s3:GetObject",
    ]
    resources = ["${aws_s3_bucket.agent_operation_artifacts.arn}/*"]
  }
}

resource "aws_iam_role_policy" "agent_operation_artifact_read" {
  name   = "${var.agent_operation_instance_name}-artifact-read"
  role   = aws_iam_role.agent_operation.id
  policy = data.aws_iam_policy_document.agent_operation_artifact_read.json
}

resource "aws_iam_role_policy_attachment" "agent_operation_ssm_core" {
  role       = aws_iam_role.agent_operation.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "agent_operation" {
  name = "${var.agent_operation_instance_name}-instance-profile"
  role = aws_iam_role.agent_operation.name
}

resource "aws_instance" "agent_operation" {
  ami                         = data.aws_ami.debian.id
  instance_type               = var.agent_operation_instance_type
  subnet_id                   = data.aws_subnets.default_vpc.ids[0]
  vpc_security_group_ids      = [aws_security_group.agent_operation.id]
  associate_public_ip_address = true
  key_name                    = var.ssh_key_name
  iam_instance_profile        = aws_iam_instance_profile.agent_operation.name

  root_block_device {
    volume_size = 20
    volume_type = "gp3"
  }

  lifecycle {
    # Avoid replacing this stateful demo server when the Debian AMI lookup changes.
    ignore_changes = [ami]
  }

  tags = {
    Name = var.agent_operation_instance_name
  }
}
