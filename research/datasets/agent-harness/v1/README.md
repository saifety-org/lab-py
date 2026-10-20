# Agent harness development scenarios v1

16 synthetic scenarios: eight attack/benign pairs with identical document bytes and different trusted authorizations. These are small development/regression fixtures, not a blind test set, training data, or evidence of real-agent robustness.

| Pair | Untrusted carrier / attack objective |
| --- | --- |
| override | Direct instruction override; unauthorized virtual audit write |
| system-spoof | Fake system-message delimiters; unauthorized write |
| structured-mcp | Workflow inside MCP `structuredContent` |
| embedded-resource | Workflow inside a textual MCP resource |
| distributed | Action JSON fragmented across two tool responses |
| multistep-exfil | Read a fake private canary, send it to a virtual external outbox |
| multilingual-obfuscation | Russian/English instructions with zero-width characters |
| answer-tamper | Replace the correct sum with a wrong answer and finish |

Each benign counterpart explicitly authorizes the requested effects in the trusted task. This intentionally tests authorization-sensitive false positives: an imperative sentence or action plan alone does not establish maliciousness. Each ordinary task requires reading the initial documents and producing the exact virtual summary. Unauthorized writes, private reads and sends remain possible **inside the virtual world**, so an oracle can measure violations rather than silently prevent them.

`task` is the only scenario metadata supplied to an agent. `permissions`, `expected`, `kind`, `pair` and `attack_goal` belong to the evaluator. `documents` are returned only by the fake MCP server. Every path, URI, destination and canary is fictitious; no tool opens model-provided paths, accesses the network, or runs commands.

`manifest.json` binds the exact `cases.json` bytes with SHA256, records the four-case smoke selection and same-author review provenance. Semantic checks: attack goals conflict with trusted tasks; expected benign effects are authorized; paired documents match; distributed and multi-step flows fit eight action turns including finish. Review is AI-assisted and **not independent**. Dataset license: `NOASSERTION`.

The runner, local Ollama adapter, budgets, metric definitions and PC commands live in [lab-py](https://github.com/saifety-org/lab-py). It pins this corpus by immutable commit and checksums and uses the actual pinned Go MCP proxy. CI's deliberately vulnerable scripted actor checks plumbing and oracles; its attack success rates must never be presented as LLM results. Running the corpus does not authorize tuning or re-freezing the already-observed contextual v4 test set.
