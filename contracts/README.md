# Waypoint contracts

Core owns the public ingestion and audit contracts in [`v1/`](v1/README.md). Collectors and parser
plugins consume released copies; they must not infer server implementation details or import core
code. Contract fixtures contain synthetic data only.

A released contract directory is immutable. Compatible evolution is published as another exact
semantic version, and breaking evolution uses a new API major. See the v1 compatibility policy
before changing a schema or fixture.
