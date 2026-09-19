# Getting Started

A quickstart for running menata-app locally. For "how do I define my own data model on top of
this," see [writing-guide.md](writing-guide.md) once you're up and running.

## Prerequisites

- Go 1.27 or later
- PostgreSQL (any recent version)

## 1. Clone and configure

```sh
git clone https://github.com/menata-id/menata-app.git
cd menata-app
cp .env.example .env
```

Open `.env` and set the values `.env.example` marks `CHANGE_ME`:

| Variable | What it's for |
|---|---|
| `DATABASE_URL` | Postgres connection string. Use a dedicated role, not a shared superuser — `.env.example` explains why. |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | A bootstrap credential the process refuses to start without (see below — you likely won't use it day to day). |
| `SESSION_SECRET` | Signs session cookies. Any long random string. |

Everything else in `.env.example` has a working default for local development (`PORT=4000`,
`SECURE_COOKIES=true` — set to `false` only for plain `http://localhost`, since a `Secure` cookie
is silently dropped over non-HTTPS). SMTP is optional: if `SMTP_HOST` is left empty, outbound
email (verification links, password resets) is written to the server's own log instead of sent —
fine for local development, not for production.

## 2. Run migrations

```sh
make migrate-up
```

## 3. Build and run

```sh
make run
```

This runs `templ generate` then builds and starts `cmd/server`. The server listens on `PORT`
(default `4000`).

## 4. Confirm it's running

```sh
curl http://localhost:4000/health
```

Then open `http://localhost:4000/login` in a browser.

## 5. Get in

There are two ways in, and for normal use you want the second one:

- **Self-registration** (the real path): go to `/register` and create a workspace with your own
  email and password. This is exactly how a real user gets in — it does not require the
  `ADMIN_USERNAME`/`ADMIN_PASSWORD` bootstrap credential at all. If `SMTP_HOST` isn't configured,
  find the verification link in the server's own stdout/log output (search for `[mail]`) instead
  of a real inbox.
- **The bootstrap admin credential** (`ADMIN_USERNAME`/`ADMIN_PASSWORD` from `.env`): a fallback
  identity that predates per-user accounts. It still works, but self-registration is the path
  that exercises the real Workspace/membership model — prefer it unless you specifically need the
  fallback.

Once you're in, you land on the Workspace Home and can open the pre-declared applications from
the top navigation.

## Next

The Machines you see (Task, Project, Document, ...) are declared entirely in
`metadata/*.yaml` — no Go code defines them. To add your own data model, see
[writing-guide.md](writing-guide.md).
