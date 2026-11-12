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

**Fallthrough**:
What an enabled flag serves to a user that no rule matched.
_Avoid_: Default rule, else branch

### People and access

**Admin**:
A person or tool that creates and changes flags.
_Avoid_: Operator, manager

**Admin key**:
A named secret that identifies one admin.
_Avoid_: API token, password
