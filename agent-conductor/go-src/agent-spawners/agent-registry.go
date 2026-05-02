package agentspawners

import "agent-orchestrator/agent-spawners/lib"

// agent-registry.go: registers all defined agents

func All() []lib.AgentDefinition {
	return []lib.AgentDefinition{
		LambdaGeneralPurposeAgent{},
	}
}
