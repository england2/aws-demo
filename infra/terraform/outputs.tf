output "instance_id" {
  description = "EC2 instance ID."
  value       = aws_instance.server.id
}

output "public_ip" {
  description = "Public IP address of the Debian instance."
  value       = aws_instance.server.public_ip
}

output "public_dns" {
  description = "Public DNS name of the Debian instance."
  value       = aws_instance.server.public_dns
}

output "ssh_command" {
  description = "SSH command for the Debian instance."
  value       = "ssh -i /path/to/aws-demo-key.pem admin@${aws_instance.server.public_ip}"
}

output "ami_id" {
  description = "AMI ID selected for the instance."
  value       = data.aws_ami.debian.id
}

output "ecr_repository_name" {
  description = "Name of the ECR repository."
  value       = aws_ecr_repository.cpu_spin.name
}

output "ecr_repository_url" {
  description = "Repository URL for pushing and pulling images."
  value       = aws_ecr_repository.cpu_spin.repository_url
}

output "ecr_registry_id" {
  description = "AWS registry ID that owns the ECR repository."
  value       = aws_ecr_repository.cpu_spin.registry_id
}

output "instance_role_name" {
  description = "IAM role attached to the EC2 instance."
  value       = aws_iam_role.server.name
}

output "instance_profile_name" {
  description = "IAM instance profile attached to the EC2 instance."
  value       = aws_iam_instance_profile.server.name
}

output "agent_operation_instance_id" {
  description = "EC2 instance ID for the agent-operation server."
  value       = aws_instance.agent_operation.id
}

output "agent_operation_public_ip" {
  description = "Public IP address of the agent-operation instance."
  value       = aws_instance.agent_operation.public_ip
}

output "agent_operation_public_dns" {
  description = "Public DNS name of the agent-operation instance."
  value       = aws_instance.agent_operation.public_dns
}

output "agent_operation_ssh_command" {
  description = "SSH command for the agent-operation instance."
  value       = "ssh -i /path/to/aws-demo-key.pem admin@${aws_instance.agent_operation.public_ip}"
}

output "agent_operation_role_name" {
  description = "IAM role attached to the agent-operation instance."
  value       = aws_iam_role.agent_operation.name
}

output "agent_operation_instance_profile_name" {
  description = "IAM instance profile attached to the agent-operation instance."
  value       = aws_iam_instance_profile.agent_operation.name
}

output "agent_operation_artifact_bucket_name" {
  description = "S3 bucket used to stage agent-operation deployment artifacts."
  value       = aws_s3_bucket.agent_operation_artifacts.bucket
}

output "agent_operation_events_queue_name" {
  description = "SQS queue name for agent-operation runtime events."
  value       = aws_sqs_queue.agent_operation_events.name
}

output "agent_operation_events_queue_url" {
  description = "SQS queue URL for agent-operation runtime events."
  value       = aws_sqs_queue.agent_operation_events.url
}

output "agent_operation_events_queue_arn" {
  description = "SQS queue ARN for agent-operation runtime events."
  value       = aws_sqs_queue.agent_operation_events.arn
}
