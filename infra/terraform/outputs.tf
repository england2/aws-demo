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
