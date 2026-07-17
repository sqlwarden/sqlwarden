# Licensing

SQLWarden's source code is licensed under the GNU Affero General Public
License v3.0 (see `LICENSE`), with the following exceptions:

- All source files under `enterprise/`
- All source files under `frontend/src/enterprise/`

Files under those paths are Copyright (c) SQLWarden and licensed under the
SQLWarden Enterprise License (see `enterprise/LICENSE`). They are the source
code of SQLWarden Enterprise features and are not open source.

Community builds of SQLWarden (`go build` without the `enterprise` build
tag, and frontend builds without `SQLWARDEN_EDITION=enterprise`) contain no
code from those paths.

"SQLWarden" and the SQLWarden logo are trademarks of the SQLWarden project
and are not licensed under the AGPL. You may not use them to identify
modified or derived distributions without permission.
