---
name: Bug report
about: Something is broken or behaving unexpectedly
title: ""
labels: bug
assignees: ""
---

## Summary

<!-- One sentence describing what's wrong. -->

## Steps to reproduce

1.
2.
3.

## Expected

<!-- What you expected to happen. -->

## Actual

<!-- What actually happened. Include error messages or unexpected output. -->

## Environment

- Tunnd version: <!-- output of `tunnd version` -->
- Server version: <!-- output of `tunnd-server version` -->
- OS / distribution: <!-- e.g. Ubuntu 22.04, macOS 14.5, Windows 11 -->
- Architecture: <!-- amd64 / arm64 -->
- Deployment: <!-- bare metal / docker / behind caddy / etc. -->

## Logs

<details>
<summary>Server logs</summary>

```
<!-- journalctl -u tunnd --tail=100  or  docker logs tunnd-server --tail 100 -->
```

</details>

<details>
<summary>Client logs</summary>

```
<!-- output of `tunnd http <port>` (with --log-level=debug if helpful) -->
```

</details>

## Additional context

<!-- Anything else useful: config snippets (with secrets redacted), screenshots, etc. -->
