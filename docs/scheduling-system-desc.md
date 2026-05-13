# Scheduling System Description

## Purpose

The system receives external operational messages, stores them durably in SQLite, and decides whether `main` should spawn an agent.

The scheduler's job is deliberately narrow: it returns spawn decisions. It is not a job runner, job tracker, retry engine, or lifecycle manager. After a spawn decision has been returned, the scheduler should remember that fact so the same stored message or same alarm chain does not cause another spawn decision after a restart, deploy, or repeated scheduler run.

The main goal is:

> Spawn at most one agent for a burst or chain of related alarms, while preserving enough database state to avoid duplicate spawn decisions across process restarts.

The system prefers avoiding duplicate scheduling over guaranteeing that every possible spawn eventually happens. If the program marks a chain as decided and crashes before `main` actually spawns the agent, that chain may not be scheduled later. That is acceptable for the current design because the system is explicitly not trying to provide queue/outbox semantics.

## Inputs

The scheduler is fed by durable database rows. There are two conceptual input streams:

- CloudWatch alarm messages
- Ticket messages

Both inputs are stored as raw message text, plus the AWS account number extracted from the message. The raw body should be retained because it is the payload `main` can hand to the agent or use to build the agent request.

The scheduler should not depend on in-memory history. Database state is the source of truth for what has been received and what has already produced a spawn decision.

## Output

Both alarm scheduling and ticket scheduling return the same conceptual result:

```go
type ScheduleDecision struct {
	ToSchedule    bool
	Text          string
	AccountNumber string
}
```

`ToSchedule` means `main` should spawn an agent for this decision.

`Text` is the message text the agent should receive. For alarms, this should normally be the newest alarm message in the chain. For tickets, it is the ticket message body.

`AccountNumber` identifies the AWS account the agent should operate against.

The scheduler does not spawn the agent directly. It returns decisions to `main`, and `main` owns whatever process or API call actually starts the agent.

## Core Scheduling Model

The scheduler should be thought of as an idempotent decision pass over stored messages.

Each scheduler run asks:

1. Are there alarm chains that should now cause a spawn decision?
2. Are there ticket messages that have not yet caused a spawn decision?
3. Which rows must be marked as already decided so the same decision is not returned again?

The scheduler should be able to run repeatedly. A second run over unchanged database contents should return no duplicate decisions.

The scheduler should also be able to run after a restart. If previous rows already produced decisions, that must be visible from SQLite alone.

## Alarm Scheduling Goal

CloudWatch alarms can arrive in bursts. If many related alarms arrive close together, the conductor should not spawn one agent per alarm. Instead, same-account alarms close in time should be treated as a chain, and the chain should produce one spawn decision.

For the current design, alarms are related only by AWS account number. There are no alarm configs, no named groups, and no per-alarm rules. If two alarm messages have the same AWS account number, they are candidates to be chained together. If they have different AWS account numbers, they belong to different scheduling groups.

This means the grouping rule is:

> All CloudWatch alarm messages from the same AWS account belong to the same potential alarm group.

The chaining rule is:

> Within one AWS account group, consecutive alarms are chained when each alarm arrives within one hour of the previous alarm in that same account group.

This is a rolling chain, not a fixed one-hour bucket.

Example:

```text
10:00 account A
10:50 account A
11:40 account A
```

These three alarms form one chain because:

- `10:50` is within one hour of `10:00`
- `11:40` is within one hour of `10:50`

It does not matter that `11:40` is more than one hour after `10:00`. Each link only needs to be close enough to the previous alarm in the same account.

Counterexample:

```text
10:00 account A
11:05 account A
```

These do not form a chain because the second alarm is more than one hour after the previous same-account alarm. Each message is isolated unless another same-account message appears close enough in time.

Different accounts never chain together:

```text
10:00 account A
10:10 account B
10:20 account A
```

For account A, `10:00` and `10:20` can chain. The account B message is evaluated independently and does not interrupt or join the account A chain.

## When an Alarm Chain Should Spawn

A single alarm by itself should not produce a spawn decision. The purpose of alarm scheduling is to detect repeated or clustered alarm pressure, not to spawn an agent for every individual alarm.

An alarm chain qualifies for scheduling when:

1. It contains at least two alarm messages.
2. All messages in the chain have the same AWS account number.
3. Each adjacent pair of messages is no more than one hour apart.
4. No previous scheduler run has already returned a spawn decision for that chain.

When a chain qualifies, the scheduler should return exactly one `ScheduleDecision`.

For that decision:

- `ToSchedule` should be true.
- `AccountNumber` should be the AWS account number shared by the chain.
- `Text` should be the raw message body of the newest alarm in the chain.

Using the newest message is useful because it represents the latest known alarm context for the account at the time the chain was evaluated.

## Alarm Idempotency

Alarm scheduling needs a database marker that means:

> A spawn decision has already been returned for this alarm chain.

The existing column concept for this is `returned_spawn_decision_for_chained_set`.

This is not job state. It does not mean an agent is running, succeeded, failed, timed out, or retried. It only means the scheduler has already returned a spawn decision for the chain containing that row.

Once the scheduler decides to return a spawn decision for a chain, it should mark the relevant chain rows as having had a spawn decision returned. On future scheduler runs, that same chain should not produce another decision.

The scheduler may also mark rows with `is_chained` once they are identified as belonging to a qualifying chain. This is descriptive scheduler state. It means the row was found to be part of a same-account rolling chain. It should not be interpreted as job state.

## Alarm Chain Extension Behavior

The most important subtle behavior is what happens when new alarms arrive after an earlier chain already produced a spawn decision.

Suppose the scheduler sees:

```text
10:00 account A
10:20 account A
```

That chain qualifies, so the scheduler returns one decision and marks the chain as decided.

Later, a new alarm arrives:

```text
10:00 account A  decided
10:20 account A  decided
10:45 account A  new
```

The new `10:45` alarm is within one hour of `10:20`, so conceptually it belongs to the same rolling chain. But because the chain has already had a spawn decision returned, the scheduler should not return a second decision for `10:45`. It should absorb that row into the already-decided chain by marking it as chained and decided.

This behavior is central to preventing duplicate agents during long bursts of related alarms.

A chain only becomes eligible for a new spawn decision after the rolling chain is broken by time. For example:

```text
10:00 account A
10:20 account A
10:45 account A
12:10 account A
12:30 account A
```

The first three rows are one chain. The `12:10` row is more than one hour after `10:45`, so it starts a new potential chain. When `12:30` arrives, those two later rows form a new chain that can produce a new spawn decision, assuming that later chain has not already been decided.

## Alarm Scheduling Pass

Conceptually, an alarm scheduling run should:

1. Load all alarm rows needed for scheduling.
2. Separate rows by AWS account number.
3. Sort each account's rows by receive time, with a stable tie-breaker such as row ID.
4. Walk each account's rows in order.
5. Build rolling chains where adjacent rows are no more than one hour apart.
6. Ignore chains with fewer than two rows.
7. For each qualifying chain:
   - mark rows as chained;
   - check whether any row in the chain already indicates that a spawn decision was returned;
   - if any row was already decided, mark the rest of that chain as decided and return no decision;
   - if no row was already decided, mark the chain as decided and return one spawn decision using the newest row.

This pass should produce zero or more decisions. There may be multiple decisions if multiple AWS accounts each have new qualifying chains, or if one account has multiple separate chains separated by more than one hour.

## Ticket Scheduling Goal

Ticket scheduling is simpler than alarm scheduling.

Tickets do not need grouping, chaining, or time-window logic. Each ticket message is already an explicit unit of work. If a ticket has not already caused a spawn decision, it should cause one.

The ticket rule is:

> Every ticket row that has not yet returned a spawn decision should return exactly one spawn decision.

For each undecided ticket:

- `ToSchedule` should be true.
- `Text` should be the raw ticket body.
- `AccountNumber` should be the ticket's AWS account number.

After returning that decision, the ticket row should be marked as decided so it will not schedule again.

## Ticket Idempotency

Ticket scheduling needs a database marker that means:

> This ticket row has already caused the scheduler to return a spawn decision.

The existing column concept for this is `returned_spawn_decision_for_ticket`.

Like alarm decision markers, this is not job state. It does not say whether the agent ran, succeeded, failed, or should be retried. It only prevents the scheduler from returning the same ticket decision more than once.

## What The Scheduler Does Not Do

The scheduler should not:

- track active jobs;
- track job success or failure;
- retry jobs;
- store job IDs;
- deduplicate exact duplicate SQS messages;
- decide based on CloudWatch alarm name;
- use alarm configuration rules;
- group alarms across accounts;
- maintain in-memory scheduling state that is required for correctness;
- treat `returned_spawn_decision_*` columns as job lifecycle columns.

Those omissions are intentional. Adding job state or delivery guarantees would change the system into a different design, likely requiring an outbox table, job table, lease model, or retry policy. The current system is much simpler: it stores input messages and records whether scheduling decisions have already been returned.

## Database Meaning

The database should represent durable scheduler memory, not live program state.

For alarm rows, the important durable facts are:

- when the message was received;
- the raw message text;
- the AWS account number;
- whether the row has been identified as part of a chain;
- whether the chain containing the row has already produced a spawn decision.

For ticket rows, the important durable facts are:

- when the message was received;
- the raw message text;
- the AWS account number;
- whether the ticket has already produced a spawn decision.

Raw bodies should be stored as text, not bytes, because the scheduler and test tooling are treating the message as JSON text and humans need to inspect it easily.

SQLite booleans should be represented as integers constrained to `0` or `1`.

## Transaction Semantics

A scheduler run should treat "mark rows as decided" and "return decisions" as one logical operation.

Conceptually:

1. The scheduler identifies decisions.
2. It marks the corresponding rows as already decided.
3. It commits those marks.
4. It returns the decisions to `main`.

This ordering means the database is protected against duplicate decisions even if `main` later crashes or the spawn call fails. The tradeoff is that a decision may be lost after being marked but before an agent is actually spawned. For the current system, that tradeoff is accepted.

If the future requirement changes to "never lose a spawn request," this design should be revisited. At that point, the scheduler should likely write pending work into a durable outbox or jobs table and have a separate component deliver that work.

## Relationship Between Main And Scheduler

`main` should not own the scheduling definitions or sqlc details. `main` should provide configuration, call the scheduler or database worker, receive `ScheduleDecision` values, and spawn agents based on those values.

The scheduler/database worker should own:

- reading database rows;
- inserting message rows when needed;
- applying alarm chain rules;
- applying ticket decision rules;
- marking rows as chained or decided;
- returning scheduling decisions.

`main` should own:

- program flags and configuration;
- deciding when to call the scheduler;
- printing or logging decisions in the demo;
- spawning agents in the production flow.

The shared `ScheduleDecision` type should live in an importable non-`main` package so both `main` and scheduler code can refer to the same shape without importing `package main`.

## Expected Outcomes

A fresh database with one isolated alarm should produce no alarm decision.

A fresh database with two or more same-account alarms no more than one hour apart should produce one alarm decision.

A fresh database with eight same-account alarms twelve minutes apart should produce one alarm decision, not eight.

Running the scheduler a second time over that same database should produce no decisions.

Adding another same-account alarm within one hour of the already-decided chain should still produce no new decision; it should be absorbed into the decided chain.

Adding two later same-account alarms after the previous chain has been broken by more than one hour should produce one new decision for the new chain.

Adding alarms from two accounts should evaluate each account independently.

Adding one undecided ticket should produce one ticket decision.

Running the scheduler again over that same ticket should produce no ticket decision.

Adding three undecided tickets should produce three ticket decisions, regardless of their timestamps, because tickets do not chain.
