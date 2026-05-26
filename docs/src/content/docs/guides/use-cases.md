---
title: Use Cases
description: 'Concrete things developers and teams actually use Tunnd for, with the exact commands.'
---

Tunnd does one thing: takes a port on your laptop and gives the
internet a stable URL pointing at it. Here are the workflows where
that's actually useful, with the exact commands. Pick the closest
match and adapt.

## Backend development

Anything that speaks HTTP/1.1 on a port — Go (net/http, gin, echo,
chi, fiber), Node (Express, Fastify, NestJS, Hono), Python (FastAPI,
Django, Flask), Java (Spring Boot, Quarkus), Rust (Axum, Actix), Ruby
(Rails), .NET, Elixir (Phoenix), PHP (Laravel) — works without
configuration. GraphQL over HTTP is just HTTP; subscriptions over
WebSocket are upgrade-bridged transparently. For gRPC, use the raw
TCP tunnel so the HTTP/2 wire travels untouched.

### Develop webhooks against a stable URL

Stripe, GitHub, Slack, Twilio, Shopify — all of them deliver test
webhooks to a public URL. Tunnd gives you one that doesn't change
between sessions:

```bash
tunnd http 8080 --subdomain stripe-webhooks
# → https://stripe-webhooks.tunnd.yourdomain.com
```

Configure that URL once in the upstream service's dashboard. Pin the
subdomain so you can leave the integration set up forever; Ctrl+C
when you're done, bring it back tomorrow with the same command.

### Share your local API with a teammate

Backend dev needs to hand an iOS or Android dev a real working API to
hit, without a deploy:

```bash
tunnd http 3000 -s alex-api
# → https://alex-api.tunnd.yourdomain.com
```

The mobile dev points their device at that base URL. No staging
environment to coordinate on, no merge-and-deploy roundtrip.

### Expose a database for a one-off debug session

Postgres, Redis, MongoDB, MySQL, MQTT — anything TCP. The server
allocates a public port automatically:

```bash
tunnd tcp 5432
# → tcp://tunnd.yourdomain.com:20000 → localhost:5432
```

```bash
# From the other machine:
psql "postgres://user:pass@tunnd.yourdomain.com:20000/dbname"
```

Tear it down when you're done. Don't expose production credentials
over this — see [Security Best Practices](/guides/security-best-practices).

### Tunnel a gRPC service

The pragmatic path is a raw TCP tunnel. The HTTP/2 wire travels
untouched, so unary and streaming both work:

```bash
tunnd tcp 50051
# → tcp://tunnd.yourdomain.com:20001 → localhost:50051
```

Clients connect directly to that host:port. The trade-off is a
non-443 endpoint; for development and demos that's normal.

---

## Frontend / UI development

Next.js, Vite (React, Vue, Svelte, Solid), Remix, SvelteKit, Nuxt,
Astro, Storybook — all of them are HTTP servers on a port, and HMR
rides over a WebSocket upgrade that Tunnd bridges transparently. Same
command for any of them.

### Share your dev server with a designer or PM

Designer asks "can I see it?" mid-meeting:

```bash
tunnd http 5173 -s acme-product
# → https://acme-product.tunnd.yourdomain.com
```

Hot reload still works. They watch your code update as you save.

### Test on real devices

Real iPhone, real Android, real iPad pointed at your laptop's dev
server. No cross-compile, no Vercel preview wait, no ngrok-style
session timeouts.

```bash
tunnd http 3000
# Open the printed URL on every device.
```

### Demo in a meeting without a deploy

Skip the "let me push first" step:

```bash
tunnd http 3000 -s feature-x
# Paste https://feature-x.tunnd.yourdomain.com into Slack/Zoom.
```

No CI wait, no preview deploy.

---

## Working in a team

If a few engineers self-host one Tunnd server, each gets their own
token from the admin dashboard and a personal subdomain:

```bash
# Alice runs (her token, her subdomain):
tunnd http 3000 -s alice
# → https://alice.tunnd.example.com

# Bob runs (his token, his subdomain):
tunnd http 8080 -s bob
# → https://bob.tunnd.example.com
```

The admin can revoke any token in one click when someone leaves.
Nobody shares credentials, nobody overlaps subdomains, nobody pays a
per-seat tunnel bill.

For one person running multiple services at once (frontend, API,
DB), see [Multiple Tunnels](/guides/multiple-tunnels).

---

## SSH into your laptop from anywhere

For remote debugging or screen-sharing your machine to someone
without exposing your home network:

```bash
# Start sshd locally, then:
tunnd tcp 22
# → tcp://tunnd.yourdomain.com:20000

# From the other side:
ssh -p 20000 user@tunnd.yourdomain.com
```

Pair with proper SSH keys and `fail2ban`; never expose root login
over a password.

---

## When Tunnd is the wrong choice

Honest fit notes — these workflows are better served by something
else:

- **Production traffic.** Tunnd is for development, demos, webhooks,
  and short-lived shares. Production services should run on
  production infrastructure with proper load balancing, multi-region
  failover, and observability that Tunnd doesn't provide.
- **OAuth / SSO in front of your tunnel.** ngrok and Cloudflare
  Access do this; Tunnd doesn't add auth in front of your tunneled
  service yet. You can layer it in your reverse proxy if you really
  need it.
- **Global anycast for low latency from anywhere.** Tunnd routes
  through your single VPS — pick a region close to your team.
- **HTTP/3 / QUIC, UDP, or gRPC over HTTP/2 at `:443`** end-to-end.
  See `tunnd tcp` above for gRPC; HTTP/3 and UDP are not yet on the
  feature surface.

---

## Next steps

- [Custom Subdomains](/guides/custom-subdomains) — pinning rules and tips
- [Multiple Tunnels](/guides/multiple-tunnels) — running several at once
- [Security Best Practices](/guides/security-best-practices) — when sharing externally
- [Troubleshooting](/guides/troubleshooting) — common errors and fixes
