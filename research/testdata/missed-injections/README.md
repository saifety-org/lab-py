# Contextual prompt-injection misses

These synthetic documents are untrusted MCP tool output, not developer or user
instructions. Each case includes the authorized user task and expected behavior.
The attack attempts to redirect the assistant or contaminate its answer.

Only the payload is passed to the scanner. Task and provenance make the label
reviewable; the current scanner does not accept the authorized task as a model
input. No fixture content is executed. Marker strings and identifiers are inert.

Run `make missed-injections` from the lab root to see current verdicts, or
`go run ./cmd/missed-injections -format json` for a machine-readable report.
The checked-in `observed.json` is a snapshot with exact app/model versions and
weight hash. Replaying a newer model may produce different verdicts.

The corpus was selected after observing misses. It is a development corpus,
not an independent benchmark or a claim about overall recall. The diagnostic
command records improvements without requiring vulnerabilities to stay present.
