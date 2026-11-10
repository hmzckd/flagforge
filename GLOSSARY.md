# FlagForge

FlagForge lets a team turn features of their applications on and off, for everyone or for chosen users, without
deploying new code.

## Language

### Flags

**Flag**:
A named on/off switch for one feature of a client application.
_Avoid_: Toggle, feature switch, setting

**Flag key**:
The short, permanent name that application code uses to ask about a flag, such as `new-checkout`.
_Avoid_: Flag id, flag name

### People and access

**Admin**:
A person or tool that creates and changes flags.
_Avoid_: Operator, manager

**Admin key**:
A named secret that identifies one admin.
_Avoid_: API token, password
