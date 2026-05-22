---
title: Use Cases
description: 'Concrete things developers and teams actually use Tunnd for, with the exact commands.'
---

A grounded look at where Tunnd fits in real workflows. The patterns below
are the ones that have come up in practice — pick the closest match and
adapt the commands.

## Backend development

### Stripe / GitHub / Slack webhook development

Webhooks need a public URL to POST to. Tunnd gives you a stable one
without a paid ngrok plan or constantly-changing tunnel names.

```bash
# On your laptop, with your local API on port 8080:
tunnd http 8080 --subdomain stripe-webhooks
# → https://stripe-webhooks.tunnd.yourdomain.com
```

Paste that URL into Stripe's webhook config (or GitHub's, or Slack's).
Because the subdomain is pinned, you can leave the integration configured
indefinitely. Tear the tunnel down with Ctrl+C when you're done; bring it
back the same way next time.

### Sharing your local API with a teammate

Backend dev needs to give the mobile dev a working endpoint to hit:

```bash
tunnd http 3000 -s alex-api
# → https://alex-api.tunnd.yourdomain.com
```

Mobile dev configures that as the API base URL on their device. No
deploy, no shared staging environment to coordinate on.

### One-off database access

Need a teammate or a cloud CI job to talk to your local Postgres for a
single debug session?

```bash
tunnd tcp 5432
# → tcp://tunnd.yourdomain.com:20000 → localhost:5432
```

```bash
# From the other machine:
psql "postgres://user:pass@tunnd.yourdomain.com:20000/dbname"
```

Same pattern works for Redis, MongoDB, MySQL, MQTT, anything that
speaks bytes.

---

## Frontend / UI development

### "Can I see it?" — sharing the dev server

Designer or PM asks for a quick look at your in-progress feature.

```bash
# Vite, Next.js, Astro, whatever — they all run on a port:
tunnd http 5173 -s acme-product
# → https://acme-product.tunnd.yourdomain.com
```

Hot module reload keeps working because WebSocket upgrades pass through
transparently. They see your code update as you save.

### Cross-device testing

Test your responsive layout on a real iPhone, a real Android, an iPad —
without a build, without ngrok-style timeouts, without a Vercel preview
deploy that may or may not match your local code.

```bash
tunnd http 3000
# Open the printed URL on every device.
```

### Quick demo in a meeting

Mid-meeting, share the latest version of the feature with the team:

```bash
tunnd http 3000 -s feature-x
# → paste https://feature-x.tunnd.yourdomain.com into Slack/Zoom chat
```

No CI wait, no deploy step, no "let me push first".

---

## Working in a team

If a few engineers self-host one Tunnd server together, each person gets
their own token from the admin dashboard and can claim a personal
subdomain.

```bash
# Alice runs:
tunnd http 3000 -s alice
# → https://alice.tunnd.example.com

# Bob runs (his own token, his own subdomain):
tunnd http 8080 -s bob
# → https://bob.tunnd.example.com
```

The admin can revoke any token in one click when someone leaves. Nobody
has to share credentials, nobody overlaps subdomains, nobody pays a
per-seat tunnel bill.

For multi-tunnel setups (one person running several services at once),
see the [Multiple Tunnels](/guides/multiple-tunnels) guide.

---

## SSH into your laptop

Useful for remote debugging or showing someone something on your machine
without exposing your home network.

```bash
# Start sshd locally, then:
tunnd tcp 22
# → tcp://tunnd.yourdomain.com:20000

# From the other side:
ssh -p 20000 user@tunnd.yourdomain.com
```

Pair with proper ssh keys + fail2ban; never expose root login over a
password.

---

## When Tunnd is the *wrong* choice

Honest fit notes — these workflows are better served by something else:

- **Production traffic.** Tunnd is for development, demos, webhooks, and
  short-lived shares. Production services should run on production
  infrastructure with proper load balancing, multi-region failover, and
  observability that Tunnd doesn't provide.
- **You need OAuth/SSO in front of your tunnel.** Use ngrok or
  Cloudflare Access — Tunnd doesn't add auth in front of your tunneled
  service yet. You can add it in your reverse proxy upstream of Tunnd
  if you really need it.
- **You need global anycast for low latency from anywhere.** Tunnd
  routes through your single VPS — pick a region close to your team.
- **You don't run any infrastructure and never want to.** Solo indie
  hacker who'd rather pay $0–$10/mo to ngrok than run a VPS. Totally
  reasonable. Tunnd's value is for people who already pay for a VPS or
  want to.

---

## Next steps

- [Custom Subdomains](/guides/custom-subdomains) — pinning rules and tips
- [Multiple Tunnels](/guides/multiple-tunnels) — running several at once
- [Security Best Practices](/guides/security-best-practices) — when sharing externally
- [Troubleshooting](/guides/troubleshooting) — common issues
