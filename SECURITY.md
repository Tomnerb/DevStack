# Security policy

## Supported versions

DevStack is currently an early-access project. Security fixes are applied to
the latest release and the `main` branch. Older releases are not maintained.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability or include secrets,
tokens, private container data, or personally identifiable information in an
issue.

Use GitHub's **Report a vulnerability** form in the repository's **Security**
tab. Include the affected version and platform, impact, reproduction steps,
and any suggested mitigation. If private vulnerability reporting is not yet
available, open a public issue containing only a request for a private contact
channel and no vulnerability details.

Maintainers will acknowledge a report as soon as practical, investigate it,
and coordinate disclosure and a fixed release with the reporter. Please allow
time for a fix to be prepared and distributed before publishing details.

## Release integrity

Official releases are published through the repository's release workflow.
macOS installers are Developer ID-signed and notarized when all Apple release
credentials are configured and otherwise are published as explicitly documented
ad-hoc-signed builds. Windows artifacts are Authenticode-signed when release
credentials are configured and otherwise are published as explicitly documented
unsigned builds. Automatic-update archives
must have a matching entry in the release's `SHA256SUMS`; DevStack rejects an
update when that checksum is missing or invalid.
