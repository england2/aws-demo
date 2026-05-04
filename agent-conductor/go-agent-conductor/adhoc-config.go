package main

var adhocAWSFargateSpawnConfig = AWSFargateSpawnConfig{
	Region:         "us-west-2",
	Cluster:        "ecs-cluster-agent-fargate",
	TaskDefinition: "agent-fargate",
	ContainerName:  "agent-fargate",
	Subnets: []string{
		"subnet-0097cadb66a94a14c",
		"subnet-0f27d826d1e258387",
		"subnet-072d05b5920b46b90",
		"subnet-01d067ffe823ca33c",
	},
	SecurityGroups: []string{
		"sg-0fd8bf9624d0cb702",
	},
	AssignPublicIP: true,
}

const adhocAgentFargateEventsQueueURL = "https://sqs.us-west-2.amazonaws.com/204772699175/agent-fargate-events"
