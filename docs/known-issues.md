# Verification boundaries

## MySQL integration

`tools/database.TestDatabaseConfig_Init` requires a matching live MySQL instance
and is skipped in short mode. A passing `make ci` does not prove a deployment's
MySQL connectivity or schema migration.

## Host resource replacement

Settings candidates are validated before publication. A rejected candidate does
not replace Current or extension snapshots. Preparation callbacks are application
code: earlier callbacks can have side effects before a later callback rejects
that candidate. There is no automatic transaction across external resources.
Callbacks must release failed preparations; the host coordinates committing and
retiring resource generations after borrowers finish.

Configuration watchers do not create or close process loggers. Replacing a
published logger does not transfer ownership or close the previous root.

## Shutdown budgets

A deadline ends a caller's wait, not an arbitrary application callback. Handlers
and cleanup callbacks must observe their context. Repeated listener or queue
shutdown calls can wait for work remaining after an earlier deadline.

## Historical reports

Older files under docs/migration and docs/architecture record upstream decisions
and measurements. They do not override current exported API, ownership contracts,
or current benchmark evidence. Previously recorded legacy queue and global
logger issues refer to implementations removed by the Motor Core refactor.
