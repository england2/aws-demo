A prototype agent conductor system running on AWS.

### Conductor?

As far as I know, "Agent Orchestrator" largely refers to systems that spin up multiple agents to work on closely related goals, coordinating with one another using a whole slew of tools at once (e.g Agent Mail, beads, etc.).

The noun "conductor" is mostly unclaimed in this context, and I have it to mean that this system is largely responsible for the lifecycle of multiple agents working on *separate features in separate codebases*, rather than coordinating close-quarters work between agents.

### Required packages

protobuf-devel
protoc-gen-go
protoc-gen-go-grpc
sqlc
