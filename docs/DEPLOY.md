# Deploying a public demo

The hackathon wants a URL judges can open. The image in `Dockerfile` is a read-only demo: samples
preloaded, visitors can paste a draft and check it, and can verify a receipt, but nothing else
writes. The log and the signing key live in the container and are recreated on every start, so
there is nothing to back up.

The image has not been built in the environment this was written in (no Docker daemon). Build it
once yourself before you rely on it: `docker build -t revenant-demo . && docker run -p 8080:8080 revenant-demo`.

## Render, Railway, Fly.io

Any host that builds a Dockerfile and sets `$PORT` works.

- **Render:** New, Web Service, connect the GitHub repo, runtime Docker, free instance. Leave the start command empty.
- **Railway:** New Project, Deploy from GitHub repo. It detects the Dockerfile.
- **Fly.io:** `fly launch` (accept the Dockerfile), set `internal_port = 8080` in `fly.toml`.

Free instances sleep when idle, so the first visit can take half a minute. Open the URL yourself
shortly before you submit and before judging starts.

## A private instance that can write

Remove `"-readonly"` from the `ENTRYPOINT`. There is no login. Anyone who can reach the URL can
add sources and fill the disk, so only do this behind a VPN or a password-protecting proxy.

## If you cannot host it

The rules ask for a URL to a working app. A GitHub repo with a release zip and a clear README may
be accepted (judges can run it), but a live URL is safer. State in the submission exactly how to
run it.
