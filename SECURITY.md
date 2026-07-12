# Security notes

## Local-only files

Never commit these files or directories:

- `.env` and other environment files except `.env.example`
- `auth_sessions/` and WhatsApp device credentials
- database dumps, SQLite files, logs, coverage output, and local build artifacts
- private keys, tokens, API keys, or webhook secrets

## Before the first commit

1. Copy `.env.example` to `.env`.
2. Replace every placeholder with unique local/deployment values.
3. Confirm `auth_sessions/` contains no credentials intended for sharing.
4. Review `git status --short` and `git diff --cached` before committing.

If a secret is ever committed, rotate it immediately. Removing it from a later commit does not make the secret safe.
