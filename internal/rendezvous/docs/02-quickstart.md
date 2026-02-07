# Getting Started

## Desktop app (recommended)

The easiest way to use Goop2 is the desktop application.

### 1. Build

You need [Go 1.24+](https://go.dev/) and the [Wails CLI](https://wails.io/):

```bash
git clone https://github.com/petervdpas/goop2.git
cd goop2
wails build
```

### 2. Launch

```bash
./build/bin/goop2
```

The app opens a window where you can create peers, start and stop them, and manage your sites.

### 3. Create a peer

Click **Create Peer**, give it a name, and hit create. Goop2 sets up the directory, generates a cryptographic identity, and creates a default site -- all automatically.

### 4. Start and visit

Click **Start** on your peer. The viewer opens in your browser, showing your site and any other peers on your local network.

From there you can edit your site, pick a template, or connect to a rendezvous server to find peers across the internet.

## CLI mode

If you're running on a server or prefer the terminal, you can run a peer without the desktop UI:

```bash
go build -o goop2
./goop2 peer ./peers/mysite
```

If the directory doesn't have a `goop.json` yet, one is created automatically with sensible defaults. You can edit it afterward to change your display name, connect to a rendezvous server, enable Lua scripting, etc.

See [Configuration](configuration) for the full reference.

## Rendezvous server

To run a standalone rendezvous server for peer discovery across networks:

```bash
./goop2 rendezvous ./peers/server
```

This starts a lightweight HTTP server that peers can publish their presence to. See [Connecting to Peers](connecting) for setup details.

## What's next?

- **Connect to other peers** -- See [Connecting to Peers](connecting) for mDNS and rendezvous setup.
- **Customize your config** -- See [Configuration](configuration) for the full reference.
- **Use a template** -- See [Templates](templates) to install a pre-built blog, quiz, or game.
