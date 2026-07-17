# Licensing

SQLWarden's source code is licensed under the GNU Affero General Public
License v3.0 (see `LICENSE`), with the following exceptions:

- All source files under `enterprise/`
- All source files under `frontend/src/enterprise/`

Files under those paths are Copyright (c) SQLWarden and licensed under the
SQLWarden Enterprise Source License (see `enterprise/LICENSE`). They are the
source code of SQLWarden Enterprise features and are not open source.

Community builds of SQLWarden (`go build` without the `enterprise` build
tag, and frontend builds without `SQLWARDEN_EDITION=enterprise`) contain no
code from those paths.

Official SQLWarden Enterprise executables and containers combine community
and enterprise code. Their production use is governed by a written SQLWarden
Enterprise subscription agreement and the
[Enterprise Source License](enterprise/LICENSE). The community source remains
separately available under AGPLv3; the enterprise terms do not withdraw or
replace that grant.

## Network Use And Source Availability

AGPLv3 requires operators who modify SQLWarden and make it available over a
network to offer the corresponding source code to those users. Official
community source is published at <https://github.com/sqlwarden/sqlwarden>.
Operators of modified builds are responsible for publishing the corresponding
source for their build and updating the in-product source link if necessary.

## Contributions

The Developer Certificate of Origin records that a contributor is entitled to
submit a change. The separate [Contributor License Agreement](CLA.md) grants
the project the rights needed to distribute contributions in both AGPL
community releases and commercially licensed combined products. Both are
required for external contributions; see [CONTRIBUTING.md](CONTRIBUTING.md).

"SQLWarden" and the SQLWarden logo are trademarks of the SQLWarden project
and are not licensed under the AGPL. You may not use them to identify
modified or derived distributions without permission.
