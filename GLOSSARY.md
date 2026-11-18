# FlagForge

FlagForge lets a team turn features of their applications on and off, for everyone or for chosen users, without
deploying new code.

## Language

### Flags

**Flag**:
A named on/off switch for one feature of a client application.
_Avoid_: Toggle, feature switch

**Flag key**:
The short, permanent name that application code uses to ask about a flag, such as `new-checkout`.
_Avoid_: Flag id, flag name

**Environment**:
One place where client applications run, such as `staging` or `production`. Every flag behaves independently in each
environment.
_Avoid_: Stage, tier, project

**Flag setting**:
How one flag behaves in one environment. A new flag's settings are off everywhere.
_Avoid_: Flag config, flag state

**Kill switch**:
Turning a flag off in an environment, so that every user gets the feature's old behavior at once.
_Avoid_: Rollback, disable

### Evaluation

**User**:
The person a client application is asking about, identified by a stable key and described by attributes.
_Avoid_: Customer, context, subject

**Attribute**:
A named piece of text that describes a user, such as `country` or `email`.
_Avoid_: Property, trait

**Evaluation**:
Deciding which value a flag serves to one user in one environment.
_Avoid_: Check, lookup

**Reason**:
Why an evaluation produced its value: the flag was off, a rule matched, or the evaluation fell through.
_Avoid_: Explanation, cause

**Rule**:
A list of clauses and the value to serve when all of them match a user. A setting's rules are tried in order and the
first matching rule wins.
_Avoid_: Condition, filter, segment

**Clause**:
One test on one attribute of a user, such as "country is one of TR, AZ".
_Avoid_: Condition, predicate

**Targeting**:
Serving different values to different users by using rules.
_Avoid_: Segmentation, personalisation

**Rollout**:
Serving `true` to a fixed percentage of users and `false` to the rest, with each user always landing on the same side.
_Avoid_: Canary, gradual release, sampling

**Bucket**:
The number from 0 to 9,999 that a user always gets for a given flag, used to decide which side of a rollout they are on.
_Avoid_: Slot, hash, shard

**Fallthrough**:
What an enabled flag serves to a user that no rule matched.
_Avoid_: Default rule, else branch

### Client applications

**Client application**:
An application of the team that asks FlagForge which value a flag serves, usually through the SDK.
_Avoid_: Consumer, service, customer app

**SDK**:
The library a client application embeds to download flags and evaluate them locally.
_Avoid_: Client library, agent

**SDK key**:
A secret that lets a client application read every flag setting of one environment, and nothing else.
_Avoid_: API key, client token

**Default value**:
The value a client application passes with every question and gets back whenever the SDK cannot answer, for example
before the first snapshot arrives or for an unknown flag.
_Avoid_: Fallback, fallthrough (which is a different thing)

**Snapshot**:
Every flag setting of one environment at one environment version.
_Avoid_: Dump, export, config

**Flag stream**:
A long-lived connection on which FlagForge pushes a new snapshot to a client application every time its environment
changes.
_Avoid_: Websocket, feed, subscription

**Environment version**:
A counter on an environment that goes up whenever anything in its snapshot changes.
_Avoid_: Revision, sequence

### Change history

**Version**:
A counter on a flag setting that goes up by one with every change, so an admin can prove which state they edited.
_Avoid_: Revision, generation

**Audit entry**:
A permanent record of one change to a flag: who made it, when, and the state before and after.
_Avoid_: Log line, history item, event

### People and access

**Admin**:
A person or tool that creates and changes flags.
_Avoid_: Operator, manager

**Admin key**:
A named secret that identifies one admin.
_Avoid_: API token, password
