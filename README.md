# menata-app

The runtime application that turns Menata Runtime Metadata into a running application.

This repository starts from the design concepts below (carried over from the private
`menata-runtime` design-history repository) and builds the real implementation from a clean
slate — no code is ported in from prior prototypes.

## Concepts (read in order)

1. [001-design-principles.md](001-design-principles.md)
2. [002-architecture.md](002-architecture.md)
3. [003-runtime-language.md](003-runtime-language.md)
4. [004-runtime-metadata.md](004-runtime-metadata.md)
5. [005-runtime-lifecycle.md](005-runtime-lifecycle.md)
6. [006-runtime-model.md](006-runtime-model.md)
7. [007-composable-runtime-architecture.md](007-composable-runtime-architecture.md)

## Relationship to menata-runtime

`menata-runtime` (private) holds the capability-discovery history that produced these concepts:
seven parallel prototypes, benchmarks, and the capability registry that proved out what a Menata
Runtime needs to support. That discovery phase is closed. This repository (`menata-app`) is where
active development happens going forward.
