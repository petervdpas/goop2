# Getting Started

This guide walks you through creating your first Goop2 peer and visiting it in the browser.

## 1. Download and install

Download the latest Goop2 binary for your platform from the project releases, or build from source:

```bash
git clone https://github.com/pgibler/goop2.git
cd goop2
go build -o goop2 ./cmd/goop2
```

## 2. Create a peer directory

A peer is just a folder with a `goop.json` config and a `site/` directory:

```bash
mkdir -p peers/mysite/site
```

## 3. Write a configuration file

Create `peers/mysite/goop.json`:

```json
{
  "identity": {
    "key_file": "data/identity.key"
  },
  "paths": {
    "site_root": "site"
  },
  "profile": {
    "label": "My First Site",
    "email": "you@example.com"
  },
  "viewer": {
    "http_addr": "127.0.0.1:8080"
  }
}
```

This gives your peer a display name, tells it where to find your site files, and starts the local viewer on port 8080.

## 4. Add some content

Create a simple `index.html` inside the site directory:

```bash
cat > peers/mysite/site/index.html << 'EOF'
<!DOCTYPE html>
<html>
<head><title>Hello Goop2</title></head>
<body>
  <h1>Welcome to my ephemeral site!</h1>
  <p>This page is served directly from my machine.</p>
</body>
</html>
EOF
```

## 5. Start the peer

```bash
./goop2 peer ./peers/mysite
```

On first run, Goop2 will:

- Create `data/identity.key` (your persistent cryptographic identity).
- Create `data.db` (your local SQLite database).
- Start announcing presence via mDNS on your local network.
- Open the viewer at `http://127.0.0.1:8080`.

## 6. Visit your site

Open a browser and go to `http://127.0.0.1:8080`. The viewer shows your site content and lists any other peers discovered on your network.

## What's next?

- **Connect to other peers** -- See [Connecting to Peers](connecting) for mDNS and rendezvous setup.
- **Customize your config** -- See [Configuration](configuration) for the full reference.
- **Use a template** -- See [Templates](templates) to start with a pre-built blog, quiz, or game.
