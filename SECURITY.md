# Security Policy

## Reporting a vulnerability

Please report security vulnerabilities privately, not through a public GitHub issue.

Use GitHub's built-in private reporting for this repository: go to the **Security** tab →
**Report a vulnerability**, or open a draft security advisory directly at
https://github.com/menata-id/menata-app/security/advisories/new. This opens a private channel
between you and the maintainers — nothing is visible publicly until a fix has landed and the
advisory is published.

Please include:

- A description of the vulnerability and its potential impact.
- Steps to reproduce it, or a proof of concept.
- Any affected route, file, or component you've identified.

## What to expect

- Acknowledgment of your report within 5 business days.
- An assessment of severity and, where applicable, a fix timeline, communicated through the same
  private advisory thread.
- Credit in the published advisory, if you'd like it, once the issue is resolved.

## Scope

This repository is a single, continuously-deployed application rather than a versioned library —
there is no "supported versions" table to maintain. Reports should be scoped to the code and
configuration in this repository (`menata-app`); the private `menata-runtime` and
`menata-app-document` repositories are not user-facing and are out of scope.

## Please don't

- Don't open a public issue, pull request, or discussion describing an unpatched vulnerability.
- Don't test against the production deployment in ways that could degrade service for real users
  (e.g. load testing, automated scanning at volume) — a local instance from this repository is
  the right place for that.
