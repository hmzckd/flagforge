# Store targeting rules as one JSONB document

A flag setting's rules and fallthrough are stored in JSONB columns on the `flag_settings` row instead of in separate
`rules`, `clauses` and `clause_values` tables. Rules are always read and written as a whole together with the setting,
their order matters, and no query ever looks inside them, so normalised tables would only add joins and ordering
columns. The cost is that PostgreSQL no longer checks their shape: every write goes through `eval.ValidateRules`, which
is covered by table tests and a fuzz test, and the database only guarantees that the value is valid JSON.
