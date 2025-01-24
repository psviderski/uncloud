<div align="center">
  <img src="./website/images/logo.svg" height="100" alt="Uncloud logo"/>
  <h1>Uncloud</h2>
  A lightweight Docker clustering tool for running web apps on your own servers — from cloud VMs to bare metal.<br>
  Replace Heroku and Render, no Kubernetes required.
</div>

## Quick start

1. Install Uncloud CLI:

```bash
curl -fsS https://get.uncloud.run | sh
```

2. Initialize your first machine:

```bash
uc machine init root@your-server-ip
```

3. Create a DNS A record pointing `app.example.com` to your server's IP address, then deploy your app with
   automatic HTTPS:

```bash
uc run --name my-app -p app.example.com:8000/https registry/app
```

That's it! Your app is now running and accessible at https://app.example.com ✨

## Project status

Uncloud is an open source project I'm ([@psviderski](https://github.com/psviderski)) actively developing. I'd love to
share this journey with you. [Subscribe](https://uncloud.run/) to follow the progress, get early insights into new
features, and be the first to know when it's ready for production use.

I'd also love your input:

- 🐛 Found a bug? [Open an issue](https://github.com/psviderski/uncloud/issues)
- 💡 Have ideas? [Join the discussion](https://github.com/psviderski/uncloud/discussions)

## Motivation

TBD
