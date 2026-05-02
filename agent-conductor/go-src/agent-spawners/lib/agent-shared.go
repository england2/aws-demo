package lib

// agent-shared.go: interface and shared structs for a variety of agent definitions

type SQSMessage struct {
	MessageID     string
	ReceiptHandle string
	Body          []byte
	RawBody       string
}


// ai-- everytime an agent scores points during DecideRunSQSMessage, we add a JobPoint struct to AgentMatch
// this struct contains a points integer in addition to the reason why we added the point. this helps with observability into why an agent spawned!
// PointReason will be "read only", and every time we run  be defined inline in the DecideRunSQSMessage message
type JobPoint struct {
	int Points
	string PointReason
}

type AgentMatch struct {
	AgentName string
	// ArrJobPoints: the "score" that an agent-spawner uses to compete with other agent-spawners if
	// they are both fit for a single job.
	//
	//
	// this is useful for if we have multiple agents that are fit to run a job, but we only want a single agent to run on it
	// the job of the function DecideRunSQSMessage is to increase JobPriority.
	//
	// we will register multiple agents. my plan is that
	ArrJobPoints []JobPoints
}


type AgentDefinition interface {
	Name() string
	// DecideRunSQSMessage: this function consumes a message of any shape from the main SQS queue. Throughout the function body, the agent-spawner will have custom logic that gives it more points.
	DecideRunSQSMessage(message SQSMessage) (AgentMatch, error)
}
