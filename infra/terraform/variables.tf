variable "aws_region" {
  description = "AWS region to deploy into."
  type        = string
  default     = "us-west-2"
}

variable "instance_name" {
  description = "Name tag for the EC2 instance."
  type        = string
  default     = "hello-debian-server"
}

variable "instance_type" {
  description = "EC2 instance type."
  type        = string
  default     = "t3.micro"
}

variable "ssh_key_name" {
  description = "Name of an existing EC2 key pair for SSH access."
  type        = string
}

variable "ssh_allowed_cidr" {
  description = "CIDR allowed to SSH to the instance. Use your current public IP with /32."
  type        = string
}

variable "debian_release" {
  description = "Debian release to use for the official AMI lookup."
  type        = string
  default     = "12"
}

variable "debian_ami_owner" {
  description = "Official Debian AWS account ID for AMI publication."
  type        = string
  default     = "136693071363"
}

variable "ecr_repository_name" {
  description = "Name of the ECR repository for cpu-spin."
  type        = string
  default     = "cpu-spin"
}

variable "ecr_image_tag_mutability" {
  description = "Whether image tags in ECR can be overwritten."
  type        = string
  default     = "MUTABLE"
}

variable "ecr_scan_on_push" {
  description = "Whether ECR should scan images on push."
  type        = bool
  default     = true
}

variable "ecr_keep_image_count" {
  description = "How many images to retain in the ECR lifecycle policy."
  type        = number
  default     = 10
}
