# Secrets

Secrets allow you to keep **sensitive data** for your services out of your Compose file and version control. This
includes database passwords, API tokens, TLS or SSH private keys.

Uncloud provides an extended support for
[Compose secrets](https://github.com/compose-spec/compose-spec/blob/main/09-secrets.md), allowing you to fetch secret
values at deploy time from your password manager, a cloud secrets store, or local files.

## How it works

Two things go into your Compose file:

1. Define a secret in the top-level `secrets` section. This tells Uncloud how to get the secret value.
2. Reference that secret value in a service's `environment` using the `secret://<name>` format. This tells Uncloud to
   resolve the secret and set the environment variable to its value at deploy time.

```yaml title="compose.yaml"
services:
  api:
    image: myapp:latest
    environment:
      DB_PASSWORD: secret://db_password
      TLS_KEY: secret://tls_key

secrets:
  db_password:
    # Read the secret value from 1Password by running this command locally.
    x-command: op read "op://prod/myapp/db_password"
  tls_key:
    # Read the secret value from a local file. Do NOT commit this file to the repository.
    file: ./secrets/tls_key.pem
```

When you run `uc deploy`, it sees each `secret://<name>` reference, resolves the secret on your local machine, and sets
the environment variable to its value. For example, it runs the command for `db_password` and reads the file for
`tls_key`. The values never land in your Compose file or git. They go straight into the deployment.

Only secrets you reference in the services that you currently deploy get resolved. A secret referenced in multiple
places or by several services resolves only once, and the value is reused.

Secrets are resolved after building images (if any) and before preparing the deployment plan.

:::info note

Secrets can only be used to set environment variables in your services. \
Mounting a secret as a file inside the container is not supported yet. For mounting config files,
see [Configs](7-configs.md).

:::

## Secret sources

Uncloud supports multiple sources for fetching secret values. Choose the one for each secret that fits your workflow.

### Arbitrary command

The `x-command` extension runs a command and uses its output as the secret value. It's the most versatile option that
lets you pull a secret from a system Keychain, password managers, cloud services and secrets stores, or any CLI tool you
already use.

```yaml title="compose.yaml"
secrets:
  # 1Password
  db_password:
    x-command: op read "op://prod/myapp/db_password"
  # Bitwarden
  api_token:
    x-command: bw get password api-token
  # Infisical
  smtp_password:
    x-command: infisical secrets get --env=prod SMTP_PASSWORD --plain --silent
  # AWS Secrets Manager
  stripe_key:
    x-command: aws secretsmanager get-secret-value --secret-id stripe --query SecretString --output text
```

The command runs on the machine where you run `uc deploy`, in the same directory as your Compose file and with the same
environment. So it uses the CLIs and credentials you are already logged into.

If your password manager needs you to unlock it or touch a hardware key, the prompt shows up right in your terminal.
Uncloud never stores your provider credentials.

A few details worth knowing:

- The command runs directly without a shell. To use pipes or variables, call a shell yourself: `sh -c 'cmd1 | cmd2'`.
- A single trailing newline is trimmed from the output, because most CLI tools add one. Everything else is kept as is.
- The command has 1 minute to finish before `uc` times out and aborts the deployment.

`x-command` is a short form for `driver: exec`. Use the long form if you prefer to be explicit:

```yaml title="compose.yaml"
secrets:
  db_password:
    driver: exec
    driver_opts:
      command: op read "op://prod/myapp/db_password"
```

:::tip

To pull secrets from many providers through a single CLI, or to keep **encrypted secrets in your git repository**,
consider [fnox](https://fnox.jdx.dev/). It works with AWS, GCP, Azure, HashiCorp Vault, 1Password, Bitwarden, and more,
and prints a plain value you can read with `x-command`:

```yaml title="compose.yaml"
secrets:
  db_password:
    x-command: fnox get DB_PASSWORD
```

:::

### File

Read the content of a local file as the secret value. The file path is relative to the Compose file location.

```yaml title="compose.yaml"
secrets:
  db_password:
    file: ./secrets/db_password.txt
```

The file content is used verbatim without trimming any whitespaces.

:::warning

Do NOT commit these files to your repository and ensure they are protected with proper file permissions. For example,
make the file readable only by your user with `chmod 600 ./secrets/db_password.txt`.

:::

## Things to keep in mind

- Secrets resolve on the machine where you run `uc deploy`, not on the cluster. The cluster only receives the final
  value.
- Secret values passed as environment variables are stored **unencrypted** as part of the service specification in the
  distributed cluster store.
- Docker also stores the resolved environment variables **unencrypted** in each container's configuration at
  `/var/lib/docker/containers/<id>/config.v2.json` on the machine running the container. Anyone with `root` access to
  that machine or able to run `docker inspect` can read them.

## See also

- [Configs](7-configs.md): Mount non-sensitive configuration files into your containers
- [Compose support matrix](../8-compose-file-reference/1-support-matrix.md): Which Compose features Uncloud supports
