---
title: Custom Subdomains
description: 'Pin a specific subdomain so you always get the same public URL.'
---
Pin a specific subdomain so you always get the same public URL.

## Usage

```bash
# Random subdomain (default)
tunnd http 3000
# → https://brave-river.tunnd.yourdomain.com

# Fixed subdomain
tunnd http 3000 --subdomain myapp
# → https://myapp.tunnd.yourdomain.com

# Short form
tunnd http 3000 -s myapp
```

---

## Validation rules

| Rule | Valid | Invalid |
|------|-------|---------|
| Length: 3–63 characters | `myapp`, `dev-api` | `ab`, a 64-char string |
| Characters: `a-z`, `0-9`, `-` | `my-app`, `app2` | `My_App`, `app.local` |
| No leading hyphen | `my-app` | `-app` |
| No trailing hyphen | `my-app` | `app-` |
| No consecutive hyphens | `my-app` | `my--app` |
| Not reserved | `myapp` | `www`, `api`, `admin`, `mail`, `ftp` |

**Sanitization** — the server automatically trims whitespace and lowercases your input before validation. `MyApp` becomes `myapp`.

---

## Error handling

**Subdomain already in use:**
```
✗  subdomain 'myapp' is already in use
   Try a different subdomain: tunnd http 3000 --subdomain myapp2
```

Another client has that subdomain registered. Choose a different one, or wait for the existing tunnel to close.

**Invalid subdomain:**
```
Error: invalid subdomain "my_app": subdomain contains invalid characters
```

Fix the subdomain to use only `a-z`, `0-9`, and `-`.

---

## Reserved subdomains

Default reserved list: `www`, `api`, `admin`, `mail`, `ftp`.

Server operators can extend this list in `tunnd-server.yaml`:

```yaml
reserved_subdomains:
  - www
  - api
  - admin
  - mail
  - ftp
  - staging
  - prod
```

---

## Next steps

- [Multiple Tunnels](/guides/multiple-tunnels)
- [CLI Reference](/configuration/cli-reference)
