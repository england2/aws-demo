package main

type Agent struct {
	Name               string
	AgentEventSQSQueue SQSPoller
}

func (a Agent) spawnFargateTask() {

}
