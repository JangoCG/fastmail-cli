# Authentication and configuration

`fastmail auth login` validates a JMAP token using Fastmail session discovery before saving it. The hidden prompt requires a terminal; scripts use `--token-stdin` or `FASTMAIL_TOKEN`. The default session URL is `https://api.fastmail.com/jmap/session`. The token must grant each requested feature; missing capabilities produce a permission error.

`fastmail auth status` checks the current token. `fastmail auth logout` removes locally stored API and CalDAV credentials. It does not revoke remote tokens or unset environment variables. With FASTMAIL_NO_KEYRING=1 it removes file credentials only. Revoke a token in Fastmail settings when needed.

`fastmail auth caldav --username alice@example.com` reads an app password from stdin, checks calendar discovery, and stores it separately. `FASTMAIL_DAV_USERNAME` and `FASTMAIL_DAV_PASSWORD` can supply temporary credentials without saving them.

Configuration is stored under `$XDG_CONFIG_HOME/fastmail-cli`, or the OS user configuration directory (on macOS, `~/Library/Application Support/fastmail-cli`). Only global config is loaded. Config and fallback credential files are written atomically with mode 0600 in a directory created with mode 0700. Credentials use keyring service `fastmail-cli`, scoped by endpoint, with private-file fallback. `FASTMAIL_NO_KEYRING=1` forces file storage. Token values are never included in output.

`FASTMAIL_TOKEN` overrides stored tokens. `FASTMAIL_SESSION_URL` and `--session-url` override the configured discovery endpoint; stored tokens are scoped to that endpoint. `FASTMAIL_ACCOUNT` and `--account` select a JMAP account. Choose opaque account IDs from `fastmail accounts`.

```sh
fastmail config show
fastmail config set account ACCOUNT_ID
fastmail config set identity alice@example.com
fastmail config set session-url https://api.fastmail.com/jmap/session
fastmail config set dav-url https://caldav.fastmail.com/
fastmail config set dav-username alice@example.com
fastmail doctor
```

HTTPS is required. HTTP is permitted only for loopback test servers. HTTP redirects are not followed with credentials. Custom session and DAV endpoints are explicit trust decisions. There is no automatic discovery from files in the current directory.
