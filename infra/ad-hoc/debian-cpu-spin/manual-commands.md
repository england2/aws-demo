# description
commands to run on debian-cpu-spin server for setup

# login to container registry for devops
aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin 204772699175.dkr.ecr.us-west-2.amazonaws.com

# rsync
sudo apt install -y rsync
