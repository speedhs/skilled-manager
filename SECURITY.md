# Security policy

## What this program is allowed to change

Skills Manager writes only under the canonical skill directory, the config directory (`~/.agent-skills-manager` by default, including `state.json`, `targets.json`, the lock, and backups), and deployment paths it created. A deployment path is one the process can prove it owns: a symlink whose target is the canonical skill, or a copy or TOML file whose `state.json` identity matches. Anything else is left in place.

The program does not execute files under a skill's `scripts/` directory. It does not open network connections. Skill contents are data.

## Reporting a vulnerability

Please use [GitHub private vulnerability reporting](https://github.com/speedhs/skilled-manager/security/advisories/new) for this repository. If that is unavailable, open a private issue only long enough to ask for an advisory, and do not include a proof of concept that deletes files.

Include the operating system, the command you ran, and whether a file you did not expect was changed. Please give the maintainers time to ship a fix before publishing details.

## Supported versions

Security fixes go to the default branch. Until `v1`, there are no supported release branches. Upgrade by installing a newer release.
