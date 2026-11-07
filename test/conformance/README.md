# Conformance

These scripts verify protocol-facing boundaries against upstream suites.
They are not part of `go test ./...` because they need Bun, uv, network access,
and temporary checkouts.

## OpenResponses

```bash
bash test/conformance/openresponses.sh
```

The script pins the upstream commit and starts the self-contained Relay.

## ACP

```bash
bash test/conformance/acp.sh
```

The script builds the ACP example and runs the official ACP v1 TCK.

Both suites must pass before a release that changes protocol conversion or ACP
event mapping.

