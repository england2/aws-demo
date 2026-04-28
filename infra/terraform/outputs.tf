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
