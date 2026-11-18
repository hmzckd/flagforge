# PostgreSQL LISTEN/NOTIFY between instances

When a flag changes, the transaction that writes it also calls `pg_notify('flag_changes', '<environment>')`, and every
FlagForge instance keeps one connection that `LISTEN`s on that channel and refreshes its streams when a notification
arrives. PostgreSQL delivers the notification only when the transaction commits, so "the change was saved but nobody
was told" cannot happen and no outbox is needed. Notifications are not stored: a listener that is disconnected misses
them, so after every (re)connect it reloads every environment that has subscribers. We chose this over Redis pub/sub,
which would add a second system and a gap between committing the change and publishing the message (the problem an
outbox solves), and over Kafka, whose stored, replayable log we do not need because every event is a full snapshot.
Polling the environment version every second would be simpler but slower and would load the database even when nothing
changes.
