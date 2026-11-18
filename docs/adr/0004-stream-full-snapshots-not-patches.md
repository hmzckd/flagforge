# Stream full snapshots, not patches

Every event on `/sdk/v1/stream` carries the whole snapshot of the environment, not just the flag that changed. A full
snapshot is self-healing: a client that missed an event, or reconnected, is fully up to date after the next one, so
there is no sequence tracking, no "I missed something, resend" protocol and no risk of applying patches in the wrong
order. It also lets the hub keep only the newest event for a slow client and drop older ones. The cost is bandwidth:
each change sends every flag of the environment, which is fine for the tens or hundreds of flags this service targets
but would need patches or per-flag events for environments with thousands of large flags.
