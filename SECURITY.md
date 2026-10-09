# Security policy

## Reporting a vulnerability

Please report security vulnerabilities privately through GitHub: open the repository's **Security** tab and choose **Report a vulnerability** (GitHub private vulnerability reporting / security advisories), or go to https://github.com/RedHiwiK/HiwiKInsight/security/advisories/new.

Do not open a public issue or pull request for a vulnerability.

Please include:

- The affected version (`hiwikinsight version`) and deployment type (Docker image, release binary, source build).
- A description of the issue and its impact.
- Steps to reproduce or a proof of concept.
- Any suggested fix.

You can expect an acknowledgement within a few days. Once a fix is ready, a release and a security advisory will be published, crediting you unless you prefer otherwise.

## Supported versions

Security fixes are made for the latest release. Upgrade to the newest version to receive them.

## Scope

In scope, for example:

- Authentication or session bypass on the dashboard, admin or query endpoints.
- Any way to modify data through the query API (`/v1/query/*`) or the MCP server.
- Acceptance of forged App Store Server Notifications.
- Ingestion issues that allow resource exhaustion beyond the documented limits, or storage of client IP addresses.
- Path traversal or arbitrary file access through the dashboard or config handling.

Out of scope: issues that require a compromised server or config file, weak passwords or tokens chosen by the operator, missing HTTPS in deployments that ignore the documented setup, and denial of service by volume alone.

## Hardening guidance

See the security checklist in [docs/deployment.md](docs/deployment.md#security-checklist).
